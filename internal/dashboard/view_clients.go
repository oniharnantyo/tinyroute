package dashboard

import "github.com/oniharnantyo/tinyroute/internal/clients"

type ClientCard struct {
	ID               string
	Name             string
	Dialect          string
	StatusState      string // "connected", "not_configured", "not_installed"
	StatusBadgeClass string
	StatusLabel      string
}

type ClientsPageData struct {
	Clients []ClientCard
}

func buildClientCard(c clients.Client) ClientCard {
	st, _ := c.Detect()
	state := "not_installed"
	badgeClass := "bg-slate-500/10 text-slate-400 border-slate-500/20"
	label := "Not Installed"

	if st.PointedAtTinyRoute {
		state = "connected"
		badgeClass = "bg-emerald-500/10 text-emerald-400 border-emerald-500/30 font-medium"
		label = "Connected"
	} else if st.Installed {
		state = "not_configured"
		badgeClass = "bg-amber-500/10 text-amber-400 border-amber-500/30 font-medium"
		label = "Not Configured"
	}

	return ClientCard{
		ID:               c.ID(),
		Name:             c.Name(),
		Dialect:          c.Dialect(),
		StatusState:      state,
		StatusBadgeClass: badgeClass,
		StatusLabel:      label,
	}
}
