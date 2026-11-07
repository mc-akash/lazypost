package ui

import "fmt"

type Panel int

const (
	PanelEnvs Panel = iota + 1
	PanelCollections
	PanelHistory
	PanelRequest
	PanelResponse
	PanelVars
)

var panelNames = map[Panel]string{
	PanelEnvs:        "Environments",
	PanelCollections: "Collections",
	PanelHistory:     "History",
	PanelRequest:     "Request",
	PanelResponse:    "Response",
	PanelVars:        "Variables",
}

var (
	cyclePanels = []Panel{PanelEnvs, PanelCollections, PanelHistory, PanelRequest, PanelResponse}
	sidePanels  = []Panel{PanelEnvs, PanelCollections, PanelHistory}
	mainPanels  = []Panel{PanelRequest, PanelResponse}
)

func (p Panel) String() string {
	if name, ok := panelNames[p]; ok {
		return name
	}
	return fmt.Sprintf("Panel(%d)", int(p))
}

func (p Panel) isSide() bool { return p == PanelEnvs || p == PanelCollections || p == PanelHistory }

func (p Panel) isMain() bool { return p == PanelRequest || p == PanelResponse }

func (p Panel) cycleColumn(delta int) Panel {
	column := sidePanels
	if p.isMain() {
		column = mainPanels
	}
	for i, c := range column {
		if c == p {
			n := len(column)
			return column[((i+delta)%n+n)%n]
		}
	}
	return p
}
