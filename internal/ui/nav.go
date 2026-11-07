package ui

func edge(n int, bottom bool) int {
	if bottom {
		return max(0, n-1)
	}
	return 0
}

func (m Model) jumpEdge(bottom bool) Model {
	switch m.focus {
	case PanelEnvs:
		m.environments.cursor = edge(len(m.ws().Envs), bottom)
	case PanelCollections:
		m.collections.cursor = edge(len(m.collections.rows(m.ws().Collections)), bottom)
		m.onScratch = false
	case PanelHistory:
		m.history.cursor = edge(len(m.history.entries), bottom)
		m.onScratch = false
	case PanelVars:
		m.variables.cursor = edge(len(m.varRows()), bottom)
	case PanelRequest:
		m.request.row = edge(m.reqRowCount(), bottom)
	}
	return m
}
