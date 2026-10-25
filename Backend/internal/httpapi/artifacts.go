package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CodewithMubasher/Calabrass/backend/internal/agent"
	"github.com/CodewithMubasher/Calabrass/backend/internal/store"
	"github.com/xuri/excelize/v2"
)

// Artifacts are the files in a workspace, listed from disk as they are rather
// than remembered in the database. A file the agent writes is one more entry in
// a folder, so the folder is the truth: nothing has to be recorded, nothing can
// go stale, and a file removed outside the app leaves the list by itself.
//
// The rail never polls. One stream per open rail re-reads the workspace and
// re-sends the listing whenever anything about it changed, which is what makes
// the list live without a loading state — there is always a previous listing to
// show, and the first one arrives with the connection.

const (
	// artifactPollInterval is how often a workspace is looked at. Small enough
	// that a file the agent has just written is on screen by the time its reply
	// lands.
	artifactPollInterval = 400 * time.Millisecond

	// artifactListMax bounds one listing, so a runaway folder cannot flood the
	// rail or the wire.
	artifactListMax = 500

	// artifactPreviewMaxBytes is how much of a file the detail view receives.
	// It is the same ceiling read_file uses: past it the model gets a hint to
	// read around the file, and the viewer says so instead.
	artifactPreviewMaxBytes = 32 * 1024

	// artifactSniffBytes is how much of a file is read to tell text from binary.
	artifactSniffBytes = 512
)

// artifactSkipDirs never reach the rail. They are dependencies and build output:
// not artifacts, and enough of them would bury the ones that are.
var artifactSkipDirs = map[string]bool{
	".git":         true,
	".next":        true,
	".cache":       true,
	".crew":        true, // crew work has its own rail
	".venv":        true,
	"venv":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"target":       true,
	"__pycache__":  true,
}

var (
	// errArtifactPath is what a caller sees for a path that is empty, a folder,
	// or outside the workspace. The reason is logged, not returned: the UI only
	// needs to know the file cannot be reached.
	errArtifactPath = errors.New("that is not a file in this workspace")

	// errArtifactsUnavailable reports a server that booted without a usable
	// workspaces folder, or a chat with no workspace behind it.
	errArtifactsUnavailable = errors.New("the workspaces folder is unavailable; check the server log")
)

// artifactPayload is one file, as the rail lists it. The bytes stay on disk: the
// detail asks for them when it opens.
type artifactPayload struct {
	// Path is relative to the workspace, with forward slashes.
	Path string `json:"path"`
	// Name is the file's own name, extension included.
	Name string `json:"name"`
	// Extension is upper case without the dot, empty when the file has none.
	Extension string `json:"extension"`
	Size      int64  `json:"size"`
	Modified  int64  `json:"modified"` // unix milliseconds
}

// artifactSnapshot is what one stream event carries.
type artifactSnapshot struct {
	Files []artifactPayload `json:"files"`
}

// scanArtifacts lists the files in a workspace, newest first, and returns a
// signature of everything the listing shows. A caller watching for changes only
// has to compare signatures: it never reads a file's contents to do it.
func scanArtifacts(root string) ([]artifactPayload, string) {
	files := make([]artifactPayload, 0, 32)
	var signature strings.Builder

	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			// A folder that cannot be read is not a reason to fail the listing.
			return nil
		}
		if entry.IsDir() {
			if path != root && artifactSkipDirs[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		// Symlinks, sockets and the like are not files to hand over.
		if !entry.Type().IsRegular() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		name := entry.Name()
		files = append(files, artifactPayload{
			Path:      rel,
			Name:      name,
			Extension: strings.TrimPrefix(strings.ToUpper(filepath.Ext(name)), "."),
			Size:      info.Size(),
			Modified:  info.ModTime().UnixMilli(),
		})
		fmt.Fprintf(&signature, "%s\x00%d\x00%d\n", rel, info.Size(), info.ModTime().UnixMilli())

		if len(files) >= artifactListMax {
			return filepath.SkipAll
		}
		return nil
	})

	sort.Slice(files, func(i, j int) bool {
		if files[i].Modified != files[j].Modified {
			return files[i].Modified > files[j].Modified
		}
		return files[i].Path < files[j].Path
	})
	return files, signature.String()
}

// artifactsWorkspace resolves the workspace a rail is showing: the chat's own
// when it has one, the project's default otherwise. The second result names it,
// which is what download-all puts on the archive.
func (s *Server) artifactsWorkspace(ctx context.Context, userID, chatID string) (*agent.Workspace, string, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		ws := s.workspaceForUser(userID, store.Chat{})
		if ws == nil {
			return nil, "", errArtifactsUnavailable
		}
		return ws, agent.DefaultWorkspaceName, nil
	}

	chat, err := s.store.GetChat(ctx, chatID, userID)
	if err != nil {
		return nil, "", err
	}
	ws := s.workspaceForUser(userID, chat.Chat)
	if ws == nil {
		return nil, "", errArtifactsUnavailable
	}
	label := agent.DefaultWorkspaceName
	if dir := strings.TrimSpace(chat.WorkspaceDir); dir != "" {
		if filepath.IsAbs(dir) {
			label = filepath.Base(dir)
		} else {
			label = dir
		}
	}
	return ws, label, nil
}

// artifactFile resolves the ?path= argument of a file request. Every path goes
// through the workspace, so the rail cannot reach outside the folder the agent
// itself is confined to.
func (s *Server) artifactFile(r *http.Request, userID string) (string, artifactPayload, error) {
	ws, _, err := s.artifactsWorkspace(r.Context(), userID, r.URL.Query().Get("chatId"))
	if err != nil {
		return "", artifactPayload{}, err
	}

	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	if rel == "" {
		return "", artifactPayload{}, errArtifactPath
	}

	abs, err := ws.Resolve(rel)
	if err != nil {
		s.log.Warn("refused an artifact path", "path", rel, "error", err)
		return "", artifactPayload{}, errArtifactPath
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", artifactPayload{}, err
	}
	if info.IsDir() {
		return "", artifactPayload{}, errArtifactPath
	}

	name := info.Name()
	return abs, artifactPayload{
		Path:      rel,
		Name:      name,
		Extension: strings.TrimPrefix(strings.ToUpper(filepath.Ext(name)), "."),
		Size:      info.Size(),
		Modified:  info.ModTime().UnixMilli(),
	}, nil
}

// artifactStatus maps a failed file lookup onto a status code.
func artifactStatus(err error) int {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, os.ErrNotExist):
		return http.StatusNotFound
	case errors.Is(err, errArtifactsUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, errArtifactPath):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// handleArtifactStream follows one workspace and re-sends its listing whenever
// anything in it changes, so the rail is live rather than polled. The first
// snapshot goes out with the connection, which is what keeps opening the rail
// free of a loading state.
func (s *Server) handleArtifactStream(w http.ResponseWriter, r *http.Request) {
	ws, _, err := s.artifactsWorkspace(r.Context(), currentUser(r.Context()).ID, r.URL.Query().Get("chatId"))
	if err != nil {
		writeError(w, artifactStatus(err), err)
		return
	}

	stream, err := newSSEWriter(w)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	ticker := time.NewTicker(artifactPollInterval)
	defer ticker.Stop()

	sent := false
	var last string
	for {
		if files, signature := scanArtifacts(ws.Root()); !sent || signature != last {
			sent, last = true, signature
			if err := stream.event("artifacts", artifactSnapshot{Files: files}); err != nil {
				return
			}
		}

		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

// handleArtifactContent sends a file's text for the detail view. A binary file
// answers with that fact instead of its bytes, and a long file is cut at the
// preview ceiling with a flag saying so.
func (s *Server) handleArtifactContent(w http.ResponseWriter, r *http.Request) {
	abs, payload, err := s.artifactFile(r, currentUser(r.Context()).ID)
	if err != nil {
		writeError(w, artifactStatus(err), err)
		return
	}

	// Special handling for Excel files: extract sheets as CSV/JSON
	if isExcelFile(payload.Name) {
		s.handleExcelContent(w, abs, payload)
		return
	}

	file, err := os.Open(abs)
	if err != nil {
		writeError(w, artifactStatus(err), err)
		return
	}
	defer func() { _ = file.Close() }()

	limit := int64(artifactPreviewMaxBytes)
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	truncated := int64(len(data)) > limit
	if truncated {
		data = data[:limit]
	}

	head := data
	if len(head) > artifactSniffBytes {
		head = head[:artifactSniffBytes]
	}
	binary := bytes.IndexByte(head, 0) >= 0

	writeJSON(w, http.StatusOK, map[string]any{
		"binary":    binary,
		"content":   string(data),
		"truncated": truncated,
		"size":      payload.Size,
	})
}

func isExcelFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".xlsx" || ext == ".xls"
}

func (s *Server) handleExcelContent(w http.ResponseWriter, abs string, payload artifactPayload) {
	f, err := excelize.OpenFile(abs)
	if err != nil {
		s.log.Warn("could not open excel file for preview", "path", abs, "error", err)
		// Fall back to binary
		writeJSON(w, http.StatusOK, map[string]any{
			"binary":    true,
			"content":   "",
			"truncated": false,
			"size":      payload.Size,
		})
		return
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"binary":    true,
			"content":   "",
			"truncated": false,
			"size":      payload.Size,
		})
		return
	}

	type SheetData struct {
		Name string     `json:"name"`
		Rows [][]string `json:"rows"`
	}

	var allSheets []SheetData
	for _, sheet := range sheets {
		rows, err := f.GetRows(sheet)
		if err != nil {
			s.log.Warn("could not read sheet", "sheet", sheet, "error", err)
			continue
		}
		// Limit rows for preview
		if len(rows) > 200 {
			rows = rows[:200]
		}
		allSheets = append(allSheets, SheetData{
			Name: sheet,
			Rows: rows,
		})
	}

	// Convert to JSON string for the content field
	contentBytes, _ := json.Marshal(allSheets)

	writeJSON(w, http.StatusOK, map[string]any{
		"binary":    false,
		"content":   string(contentBytes),
		"truncated": false,
		"size":      payload.Size,
		"preview":   "table",
		"sheets":    allSheets,
	})
}

// handleArtifactDownload hands over one file as it is on disk.
func (s *Server) handleArtifactDownload(w http.ResponseWriter, r *http.Request) {
	abs, payload, err := s.artifactFile(r, currentUser(r.Context()).ID)
	if err != nil {
		writeError(w, artifactStatus(err), err)
		return
	}

	file, err := os.Open(abs)
	if err != nil {
		writeError(w, artifactStatus(err), err)
		return
	}
	defer func() { _ = file.Close() }()

	w.Header().Set("Content-Type", artifactContentType(payload.Name))
	w.Header().Set("Content-Disposition", artifactAttachment(payload.Name))
	w.Header().Set("Content-Length", strconv.FormatInt(payload.Size, 10))
	w.Header().Set("Cache-Control", "no-store")

	if _, err := io.Copy(w, file); err != nil {
		s.log.Warn("artifact download stopped early", "path", payload.Path, "error", err)
	}
}

// handleArtifactArchive packs the whole listing into one zip. Downloading the
// rail means downloading every file in it, and a browser will only start one
// download per click — so it gets one file that holds them all.
func (s *Server) handleArtifactArchive(w http.ResponseWriter, r *http.Request) {
	ws, label, err := s.artifactsWorkspace(r.Context(), currentUser(r.Context()).ID, r.URL.Query().Get("chatId"))
	if err != nil {
		writeError(w, artifactStatus(err), err)
		return
	}

	files, _ := scanArtifacts(ws.Root())

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", artifactAttachment(label+"-artifacts.zip"))
	w.Header().Set("Cache-Control", "no-store")

	archive := zip.NewWriter(w)
	for _, payload := range files {
		abs, err := ws.Resolve(payload.Path)
		if err != nil {
			continue
		}
		file, err := os.Open(abs)
		if err != nil {
			continue
		}
		header := &zip.FileHeader{Name: payload.Path, Method: zip.Deflate}
		header.Modified = time.UnixMilli(payload.Modified)
		entry, err := archive.CreateHeader(header)
		if err == nil {
			_, err = io.Copy(entry, file)
		}
		_ = file.Close()
		if err != nil {
			s.log.Warn("artifact left out of the archive", "path", payload.Path, "error", err)
		}
	}

	// The response is already streaming, so a failure here can only be logged.
	if err := archive.Close(); err != nil {
		s.log.Warn("could not close the artifacts archive", "error", err)
	}
}

// artifactContentType guesses a file's type from its name, falling back to the
// bytes-are-bytes type.
func artifactContentType(name string) string {
	if ct := mime.TypeByExtension(filepath.Ext(name)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// artifactAttachment renders a Content-Disposition for a download. It handles
// names outside ASCII the way the header expects.
func artifactAttachment(name string) string {
	return mime.FormatMediaType("attachment", map[string]string{"filename": name})
}
