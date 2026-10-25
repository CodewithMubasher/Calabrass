package main

// --- layout -----------------------------------------------------------------

// chatHeight is the number of rows above the prompt-input
// (the footer row under the prompt takes 1 more row).
func (m Model) chatHeight() int {
	h := m.height - m.prompt.Height() - 1
	if h < 1 {
		h = 1
	}
	return h
}

// chatHit says which timeline row (if any) is on a chat line.
type chatHit struct {
	msg  int // message index, -1 if none
	part int // first-part index of the group
	w    int // width of the row text
}

// chatLines renders every message. hits[i] describes line i.
//
// Rendering is cached per message (see Message.RenderCached), so this is
// cheap on redraws even when there are many messages: only the message
// that is currently streaming is re-rendered each frame.
func (m Model) chatLines() (lines []string, hits []chatHit) {
	w := m.centerWidth()
	spin := m.sp.View()
	for i, msg := range m.messages {
		hover := -1
		if i == m.hoverMsg {
			hover = m.hoverPart
		}
		ml, mh := msg.RenderCached(w, hover, spin)
		for j, l := range ml {
			lines = append(lines, l)
			if mh[j].Part >= 0 {
				hits = append(hits, chatHit{msg: i, part: mh[j].Part, w: mh[j].W})
			} else {
				hits = append(hits, chatHit{msg: -1, part: -1})
			}
		}
	}
	return lines, hits
}

// visibleRange picks which lines are on screen. scroll is how many lines the
// user has scrolled up from the bottom.
func visibleRange(n, h, scroll int) (start, end int) {
	if n <= h {
		return 0, n
	}
	if scroll > n-h {
		scroll = n - h
	}
	if scroll < 0 {
		scroll = 0
	}
	end = n - scroll
	return end - h, end
}

func (m Model) maxScroll() int {
	lines, _ := m.chatLines()
	n := len(lines) - m.chatHeight()
	if n < 0 {
		return 0
	}
	return n
}

func (m *Model) scrollBy(delta int) {
	m.scroll += delta
	if max := m.maxScroll(); m.scroll > max {
		m.scroll = max
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
}

// hitAt finds the timeline row under screen position (x, y), if any.
func (m Model) hitAt(x, y int) (chatHit, bool) {
	if m.screen != screenChat {
		return chatHit{}, false
	}
	chatH := m.chatHeight()
	if y < 0 || y >= chatH {
		return chatHit{}, false
	}

	cx := x - marginLeft // column inside the center column
	if cx < bubbleIndent {
		return chatHit{}, false
	}

	lines, hits := m.chatLines()
	start, _ := visibleRange(len(lines), chatH, m.scroll)
	idx := start + y
	if idx < 0 || idx >= len(hits) {
		return chatHit{}, false
	}

	h := hits[idx]
	if h.msg < 0 || cx >= bubbleIndent+h.w {
		return chatHit{}, false
	}
	return h, true
}

// handleClick opens or closes a timeline row.
func (m *Model) handleClick(x, y int) {
	if h, ok := m.hitAt(x, y); ok {
		p := &m.messages[h.msg].Parts[h.part]
		p.Expanded = !p.Expanded
		m.messages[h.msg].invalidateMsgRender()
	}
}

// handleHover remembers which timeline row the mouse is over.
// It is called on every mouse-motion event, so it does nothing unless the
// hovered row actually changed: that keeps the render cache hot.
func (m *Model) handleHover(x, y int) {
	if h, ok := m.hitAt(x, y); ok {
		if m.hoverMsg == h.msg && m.hoverPart == h.part {
			return
		}
		m.hoverMsg, m.hoverPart = h.msg, h.part
		return
	}
	if m.hoverMsg == -1 && m.hoverPart == -1 {
		return
	}
	m.hoverMsg, m.hoverPart = -1, -1
}
