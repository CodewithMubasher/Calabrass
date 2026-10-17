// Package tools is what this server offers the model: the ways to put something
// in front of the person who asked for it. Each one reads as the promise, and
// the desktop package as the mechanism.
package tools

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/CodewithMubasher/Calabrass/mcp/win-control/internal/desktop"
	"github.com/CodewithMubasher/Calabrass/mcp/win-control/internal/protocol"
)

// searchEngines is where a plain web search is sent, by the name the model uses
// for it.
var searchEngines = map[string]string{
	"google":     "https://www.google.com/search?q=",
	"duckduckgo": "https://duckduckgo.com/?q=",
	"bing":       "https://www.bing.com/search?q=",
}

// All is every tool this server has, in the order they are listed.
func All() []protocol.Tool {
	return []protocol.Tool{
		openApp(),
		playMusic(),
		searchWeb(),
		openURL(),
	}
}

func openApp() protocol.Tool {
	return protocol.Tool{
		Name: "open_app",
		Description: "Open an application, a file or a Windows settings page on the " +
			"user's computer, in front of them. By default the name is typed into " +
			"the Start menu's search and the top result is opened, which is how a " +
			"person opens an application they know the name of. Pass mode \"shell\" " +
			"when the exact program, path or Windows URI is known — \"notepad\", " +
			"\"C:\\Program Files\\app.exe\", \"ms-settings:\" — and it is opened " +
			"directly instead of searched for.",
		Schema: protocol.Object(map[string]any{
			"name": protocol.String("What to open: \"settings\", \"notepad\", \"Spotify\", or an exact path or Windows URI with mode shell."),
			"mode": protocol.Choice("How to open it: search types the name into the Start menu, shell opens it directly.",
				"search", "shell"),
		}, "name"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			name, err := protocol.RequiredString(args, "name", 200)
			if err != nil {
				return "", err
			}
			mode, err := protocol.OptionalString(args, "mode", "search", 20)
			if err != nil {
				return "", err
			}

			switch strings.ToLower(mode) {
			case "shell":
				if err := desktop.Open(name); err != nil {
					return "", err
				}
				return done(name + " on this computer"), nil

			case "search":
				if err := desktop.StartMenuSearch(name); err != nil {
					return "", err
				}
				return done(fmt.Sprintf("the top Start menu result for %q (Windows key, typed, Enter)", name)), nil

			default:
				return "", fmt.Errorf("nothing can be opened with mode %q: use search or shell", mode)
			}
		},
	}
}

func playMusic() protocol.Tool {
	return protocol.Tool{
		Name: "play_music",
		Description: "Play music for the person you are helping. Opens YouTube's " +
			"search results for what was asked for in their browser, where the " +
			"first result is the thing to press play on.",
		Schema: protocol.Object(map[string]any{
			"name": protocol.String("What to play: a song, an artist, an album, or a mood."),
		}, "name"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			name, err := protocol.RequiredString(args, "name", 200)
			if err != nil {
				return "", err
			}
			link := "https://www.youtube.com/results?search_query=" + url.QueryEscape(name)
			if err := desktop.Open(link); err != nil {
				return "", err
			}
			return done("YouTube's results for " + name + ": " + link), nil
		},
	}
}

func searchWeb() protocol.Tool {
	return protocol.Tool{
		Name: "search_web",
		Description: "Search the web for the person you are helping, and open the " +
			"results in their browser.",
		Schema: protocol.Object(map[string]any{
			"query": protocol.String("What to search for."),
			"engine": protocol.Choice(
				"Which search engine to use. Google unless there is a reason for another.",
				"google", "duckduckgo", "bing"),
		}, "query"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			query, err := protocol.RequiredString(args, "query", 500)
			if err != nil {
				return "", err
			}
			engine, err := protocol.OptionalString(args, "engine", "google", 20)
			if err != nil {
				return "", err
			}
			base, known := searchEngines[strings.ToLower(engine)]
			if !known {
				return "", fmt.Errorf("no search engine called %q: use google, duckduckgo or bing", engine)
			}

			link := base + url.QueryEscape(query)
			if err := desktop.Open(link); err != nil {
				return "", err
			}
			return done(fmt.Sprintf("a %s search for %s: %s", strings.ToLower(engine), query, link)), nil
		},
	}
}

func openURL() protocol.Tool {
	return protocol.Tool{
		Name: "open_url",
		Description: "Open a web address in the person's browser. Use this when " +
			"there is a page worth seeing, rather than a search worth running. It " +
			"opens web addresses only — for an application, a file or a Windows " +
			"settings page, use open_app.",
		Schema: protocol.Object(map[string]any{
			"url": protocol.String("The address to open, starting with http:// or https://."),
		}, "url"),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			raw, err := protocol.RequiredString(args, "url", 2000)
			if err != nil {
				return "", err
			}

			// Only the web is opened here. A shell would happily follow file: or a
			// bare path, and this one acts on whatever the model was talked into
			// asking for. The refusal says where to go instead, because a dead end
			// is what makes an assistant give up on a request it could have met.
			parsed, err := url.Parse(raw)
			if err != nil || parsed.Host == "" ||
				(parsed.Scheme != "http" && parsed.Scheme != "https") {
				return "", fmt.Errorf("%q is not a web address: give one starting with http:// or https://, "+
					"or use open_app for an application or a Windows settings page", raw)
			}

			if err := desktop.Open(parsed.String()); err != nil {
				return "", err
			}
			return done("the page: " + parsed.String()), nil
		},
	}
}

// done is what the model is told afterwards: what happened, and where to look.
// In a dry run nothing was opened, and saying otherwise would be a lie the model
// would go on to repeat to the person reading it. What it takes is the thing
// itself, phrased to read after both openings.
func done(what string) string {
	if desktop.DryRun() {
		return "Dry run, nothing was opened: " + what
	}
	return "Opened " + what
}
