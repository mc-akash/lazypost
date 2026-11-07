package ui

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/mc-akash/lazypost/internal/workspace"
)

const treeIndent = "  "

type treeRow struct {
	depth  int
	folder *workspace.Folder
	req    *workspace.Request
}

func (r treeRow) label() string {
	if r.folder != nil {
		return r.folder.Name
	}
	return r.req.Name
}

func (r treeRow) path() string {
	if r.folder != nil {
		return r.folder.Path
	}
	return r.req.Path
}

func (r treeRow) parent() *workspace.Folder {
	if r.folder != nil {
		return r.folder.Parent
	}
	return r.req.Parent
}

type collectionPanel struct {
	cursor    int
	collapsed map[string]bool
}

func newCollectionPanel() collectionPanel {
	return collectionPanel{collapsed: map[string]bool{}}
}

func (p collectionPanel) rows(root *workspace.Folder) []treeRow {
	if root == nil {
		return nil
	}
	var out []treeRow
	var walk func(f *workspace.Folder, depth int)
	walk = func(f *workspace.Folder, depth int) {
		for _, sub := range f.Folders {
			out = append(out, treeRow{depth: depth, folder: sub})
			if !p.collapsed[sub.Path] {
				walk(sub, depth+1)
			}
		}
		for _, r := range f.Requests {
			out = append(out, treeRow{depth: depth, req: r})
		}
	}
	walk(root, 0)
	return out
}

func (p collectionPanel) move(delta int, root *workspace.Folder) collectionPanel {
	p.cursor = step(p.cursor, delta, len(p.rows(root)))
	return p
}

func (p collectionPanel) selected(root *workspace.Folder) (treeRow, bool) {
	rows := p.rows(root)
	if c := clampCursor(p.cursor, len(rows)); c < len(rows) {
		return rows[c], true
	}
	return treeRow{}, false
}

func (p collectionPanel) toggle(root *workspace.Folder) collectionPanel {
	row, ok := p.selected(root)
	if !ok || row.folder == nil {
		return p
	}
	collapsed := make(map[string]bool, len(p.collapsed)+1)
	for k, v := range p.collapsed {
		collapsed[k] = v
	}
	if collapsed[row.folder.Path] {
		delete(collapsed, row.folder.Path)
	} else {
		collapsed[row.folder.Path] = true
	}
	p.collapsed = collapsed
	return p
}

func (p collectionPanel) reveal(root *workspace.Folder, path string) collectionPanel {
	i := slices.IndexFunc(collectionPanel{}.rows(root), func(r treeRow) bool { return r.path() == path })
	if i < 0 {
		return p
	}
	collapsed := maps.Clone(p.collapsed)
	for f := (collectionPanel{}).rows(root)[i].parent(); f != nil; f = f.Parent {
		delete(collapsed, f.Path)
	}
	p.collapsed = collapsed
	p.cursor = slices.IndexFunc(p.rows(root), func(r treeRow) bool { return r.path() == path })
	return p
}

type rowDecor func(r *workspace.Request) (method, mark string)

func (p collectionPanel) render(root *workspace.Folder, w, h int, focused bool, decor rowDecor) []string {
	rows := p.rows(root)
	cursor := clampCursor(p.cursor, len(rows))
	start, end := listWindow(len(rows), cursor, h)
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, p.renderRow(rows[i], w, i == cursor, focused, decor))
	}
	return lines
}

func (p collectionPanel) renderRow(r treeRow, w int, selected, focused bool, decor rowDecor) string {
	indent := strings.Repeat(treeIndent, r.depth)
	var prefix, rest, method string
	if r.folder != nil {
		prefix = "▾ "
		if p.collapsed[r.folder.Path] {
			prefix = "▸ "
		}
		rest = r.folder.Name
	} else {
		var mark string
		method, mark = decor(r.req)
		prefix = methodLabel(method) + " "
		rest = r.req.Name
		if mark != "" {
			rest += " " + mark
		}
	}
	if selected {
		return selectedRow(indent+prefix+rest, w, focused)
	}
	room := w - displayCells(indent+prefix)
	if room < 0 {
		return truncate(indent+prefix, w)
	}
	if r.req != nil {
		prefix = methodStyle(method).Render(prefix)
	}
	return indent + prefix + truncate(rest, room)
}

func (Model) collectionsBindings() []binding {
	return []binding{
		{keys: keys(&km.Send), run: Model.send, help: "send the selected request", hint: "send"},
		{keys: keys(enter), run: noCmd(Model.openOrToggle), help: "expand / collapse folder, open request", hint: "expand/open"},
		{keys: keys(&km.New), run: noCmd(Model.promptNewItem), help: "new request (name/ for a folder)", hint: "new"},
		{keys: keys(&km.Delete), run: noCmd(Model.deleteItem), help: "delete request / folder", hint: "delete"},
		{keys: keys(&km.EditInEditor), run: Model.openEditor, help: "edit request in $EDITOR", hint: "$EDITOR"},
		{keys: keys(&km.Save), run: noCmd(Model.writeDraft), help: "save changes to the request file", hint: "save"},
		{keys: keys(&km.SaveAs), run: noCmd(Model.saveAs), help: "save as a new request (folder/name)", hint: "save as"},
		{keys: keys(&km.CopyCurl), run: Model.copyAsCurl, help: "copy request as cURL (variables resolved)", hint: "copy cURL"},
		{keys: keys(&km.ImportCurl), run: func(m Model) (Model, tea.Cmd) {
			return m.importCurl(isEmptyFolder(m.ws().Collections)), nil
		}, help: "import cURL as a new request", hint: "import cURL"},
	}
}

func (m Model) openOrToggle() Model {
	if row, ok := m.collections.selected(m.ws().Collections); ok && row.req != nil {
		m.onScratch = false
		return m.setFocus(PanelRequest)
	}
	m.collections = m.collections.toggle(m.ws().Collections)
	return m
}

func (m Model) promptNewItem() Model {
	in := newInput(dialogNewRequest, "", submitNewItem).withHint(hintNewItem)
	m.overlay = &in
	return m
}

func (m Model) newItemParent() *workspace.Folder {
	row, ok := m.collections.selected(m.ws().Collections)
	switch {
	case !ok:
		return m.ws().Collections
	case row.folder != nil:
		return row.folder
	}
	return row.req.Parent
}

func submitNewItem(m Model, value string) (Model, tea.Cmd) {
	name := strings.TrimSpace(value)
	parent := m.newItemParent()
	if name == "" || parent == nil {
		return m, nil
	}
	var path, label string
	if folderName, isFolder := strings.CutSuffix(name, "/"); isFolder {
		folderName = strings.TrimSpace(folderName)
		if folderName == "" {
			return m, nil
		}
		f, err := m.ws().CreateFolder(parent, folderName)
		if err != nil {
			m = m.fail("create failed: " + err.Error())
			return m, nil
		}
		path, label = f.Path, f.Name
	} else {
		r, err := m.ws().CreateRequest(parent, name)
		if err != nil {
			m = m.fail("create failed: " + err.Error())
			return m, nil
		}
		path, label = r.Path, r.Name
	}
	m.collections = m.collections.reveal(m.ws().Collections, path)
	m.onScratch = false
	m.status = "created " + label
	return m, nil
}

func (m Model) deleteItem() Model {
	row, ok := m.collections.selected(m.ws().Collections)
	if !ok {
		return m
	}
	title := dialogDelRequest
	if row.folder != nil {
		title = dialogDelFolder
	}
	return m.confirmDelete(title, row.label(), func(m Model) Model {
		var err error
		if row.folder != nil {
			err = m.ws().DeleteFolder(row.folder)
		} else {
			err = m.ws().DeleteRequest(row.req)
		}
		if err != nil {
			m = m.fail("delete failed: " + err.Error())
			return m
		}
		key := m.draftKey(row.path())
		dir := key + string(filepath.Separator)
		m = m.dropDrafts(func(k string) bool { return k == key || (row.folder != nil && strings.HasPrefix(k, dir)) })
		under := row.path() + string(filepath.Separator)
		results := maps.Clone(m.results)
		maps.DeleteFunc(results, func(k string, _ sendResult) bool { return k == row.path() || strings.HasPrefix(k, under) })
		m.results = results
		m.collections = m.collections.move(0, m.ws().Collections)
		m.status = "deleted " + row.label()
		return m.persistSession()
	})
}
