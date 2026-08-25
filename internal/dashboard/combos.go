package dashboard

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/oniharnantyo/tinyroute/internal/dialect"
)

func (h *DashboardHandler) handleCombosView(w http.ResponseWriter, r *http.Request) {
	h.renderCombosViewWithDraft(w, r, WizardDraft{Step: 1, Mode: "ordered"}, false)
}

func (h *DashboardHandler) renderCombosViewWithDraft(w http.ResponseWriter, r *http.Request, draft WizardDraft, dialogOpen bool) {
	topo := h.deps.TopologyWatcher.Get()
	var combos []ComboItem
	var modelOptions []string
	var accountOptions []string
	if topo != nil {
		for _, c := range topo.Combos {
			mode := c.Mode
			if mode == "" {
				mode = "ordered"
			}
			combos = append(combos, ComboItem{
				Name:         c.Name,
				Mode:         mode,
				Members:      c.Members,
				Capabilities: c.Capabilities,
				Enabled:      c.IsEnabled(),
			})
		}
		sort.Slice(combos, func(i, j int) bool {
			return combos[i].Name < combos[j].Name
		})
		modelOptions = config.GetModelCandidates(*topo)
		accountOptions = config.GetAccountOptions(*topo)
	}

	data := CombosPageData{
		Combos:         combos,
		ModelOptions:   modelOptions,
		AccountOptions: accountOptions,
		Draft:          draft,
		DialogOpen:     dialogOpen,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	Layout("Combos", "combos", h.deps.PasswordStore.IsDefaultPassword(), CombosPage(data)).Render(r.Context(), w)
}

func (h *DashboardHandler) handleCombosWizard(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/dashboard/combos?error="+url.QueryEscape("Invalid form submission"), http.StatusSeeOther)
		return
	}

	action := r.FormValue("action")
	step, _ := strconv.Atoi(r.FormValue("step"))
	if step < 1 {
		step = 1
	}
	isEdit := r.FormValue("is_edit") == "true"
	initialName := r.FormValue("initial_name")
	name := strings.TrimSpace(r.FormValue("name"))
	mode := r.FormValue("mode")
	if mode == "" {
		mode = "ordered"
	}
	members := config.SplitAndTrim(r.FormValue("members_json"))
	caps := config.SplitAndTrim(r.FormValue("capabilities_json"))

	topo := h.deps.TopologyWatcher.Get()

	switch {
	case action == "open_create":
		h.renderCombosViewWithDraft(w, r, WizardDraft{
			Step: 1,
			Mode: "ordered",
		}, true)
		return

	case action == "open_edit":
		editName := r.FormValue("name")
		var target *config.Combo
		if topo != nil {
			for _, c := range topo.Combos {
				if c.Name == editName {
					cp := c
					target = &cp
					break
				}
			}
		}
		if target == nil {
			http.Redirect(w, r, "/dashboard/combos?error="+url.QueryEscape("Combo not found"), http.StatusSeeOther)
			return
		}
		m := target.Mode
		if m == "" {
			m = "ordered"
		}
		h.renderCombosViewWithDraft(w, r, WizardDraft{
			Step:         1,
			Name:         target.Name,
			InitialName:  target.Name,
			IsEdit:       true,
			Members:      target.Members,
			Mode:         m,
			Capabilities: target.Capabilities,
		}, true)
		return

	case action == "next_step_1":
		var existing []config.Combo
		if topo != nil {
			for _, c := range topo.Combos {
				if isEdit && c.Name == initialName {
					continue
				}
				existing = append(existing, c)
			}
		}
		if err := config.ValidateComboName(name, existing); err != nil {
			h.renderCombosViewWithDraft(w, r, WizardDraft{
				Step:         1,
				Name:         name,
				InitialName:  initialName,
				IsEdit:       isEdit,
				Members:      members,
				Mode:         mode,
				Capabilities: caps,
				Error:        err.Error(),
			}, true)
			return
		}
		h.renderCombosViewWithDraft(w, r, WizardDraft{
			Step:         2,
			Name:         name,
			InitialName:  initialName,
			IsEdit:       isEdit,
			Members:      members,
			Mode:         mode,
			Capabilities: caps,
		}, true)
		return

	case action == "back_step_1":
		h.renderCombosViewWithDraft(w, r, WizardDraft{
			Step:         1,
			Name:         name,
			InitialName:  initialName,
			IsEdit:       isEdit,
			Members:      members,
			Mode:         mode,
			Capabilities: caps,
		}, true)
		return

	case action == "add_member":
		model := strings.TrimSpace(r.FormValue("selected_model"))
		account := strings.TrimSpace(r.FormValue("selected_account"))

		if model == "" {
			h.renderCombosViewWithDraft(w, r, WizardDraft{
				Step:         2,
				Name:         name,
				InitialName:  initialName,
				IsEdit:       isEdit,
				Members:      members,
				Mode:         mode,
				Capabilities: caps,
				Error:        "Please select a model candidate",
			}, true)
			return
		}

		parts := strings.SplitN(model, ":", 2)
		if len(parts) != 2 {
			h.renderCombosViewWithDraft(w, r, WizardDraft{
				Step:         2,
				Name:         name,
				InitialName:  initialName,
				IsEdit:       isEdit,
				Members:      members,
				Mode:         mode,
				Capabilities: caps,
				Error:        "Invalid model candidate selected",
			}, true)
			return
		}
		modelProv, modelName := parts[0], parts[1]

		var composed string
		if account == "" || account == "any" {
			composed = model
		} else {
			accParts := strings.SplitN(account, "@", 2)
			if len(accParts) != 2 {
				h.renderCombosViewWithDraft(w, r, WizardDraft{
					Step:         2,
					Name:         name,
					InitialName:  initialName,
					IsEdit:       isEdit,
					Members:      members,
					Mode:         mode,
					Capabilities: caps,
					Error:        "Invalid connection selected",
				}, true)
				return
			}
			accProv, accName := accParts[0], accParts[1]
			if accProv != modelProv {
				h.renderCombosViewWithDraft(w, r, WizardDraft{
					Step:         2,
					Name:         name,
					InitialName:  initialName,
					IsEdit:       isEdit,
					Members:      members,
					Mode:         mode,
					Capabilities: caps,
					Error:        fmt.Sprintf("Connection %q belongs to provider %q, but model %q is from provider %q", accName, accProv, modelName, modelProv),
				}, true)
				return
			}
			composed = fmt.Sprintf("%s@%s:%s", modelProv, accName, modelName)
		}

		for _, m := range members {
			if m == composed {
				h.renderCombosViewWithDraft(w, r, WizardDraft{
					Step:         2,
					Name:         name,
					InitialName:  initialName,
					IsEdit:       isEdit,
					Members:      members,
					Mode:         mode,
					Capabilities: caps,
					Error:        fmt.Sprintf("Member %q is already in the combo", composed),
				}, true)
				return
			}
		}

		members = append(members, composed)
		h.renderCombosViewWithDraft(w, r, WizardDraft{
			Step:         2,
			Name:         name,
			InitialName:  initialName,
			IsEdit:       isEdit,
			Members:      members,
			Mode:         mode,
			Capabilities: caps,
		}, true)
		return

	case strings.HasPrefix(action, "move_up_"):
		idxStr := strings.TrimPrefix(action, "move_up_")
		idx, err := strconv.Atoi(idxStr)
		if err == nil && idx > 0 && idx < len(members) {
			members[idx-1], members[idx] = members[idx], members[idx-1]
		}
		h.renderCombosViewWithDraft(w, r, WizardDraft{
			Step:         2,
			Name:         name,
			InitialName:  initialName,
			IsEdit:       isEdit,
			Members:      members,
			Mode:         mode,
			Capabilities: caps,
		}, true)
		return

	case strings.HasPrefix(action, "move_down_"):
		idxStr := strings.TrimPrefix(action, "move_down_")
		idx, err := strconv.Atoi(idxStr)
		if err == nil && idx >= 0 && idx < len(members)-1 {
			members[idx+1], members[idx] = members[idx], members[idx+1]
		}
		h.renderCombosViewWithDraft(w, r, WizardDraft{
			Step:         2,
			Name:         name,
			InitialName:  initialName,
			IsEdit:       isEdit,
			Members:      members,
			Mode:         mode,
			Capabilities: caps,
		}, true)
		return

	case strings.HasPrefix(action, "remove_"):
		idxStr := strings.TrimPrefix(action, "remove_")
		idx, err := strconv.Atoi(idxStr)
		if err == nil && idx >= 0 && idx < len(members) {
			members = append(members[:idx], members[idx+1:]...)
		}
		h.renderCombosViewWithDraft(w, r, WizardDraft{
			Step:         2,
			Name:         name,
			InitialName:  initialName,
			IsEdit:       isEdit,
			Members:      members,
			Mode:         mode,
			Capabilities: caps,
		}, true)
		return

	case action == "next_step_2":
		if len(members) < 1 {
			h.renderCombosViewWithDraft(w, r, WizardDraft{
				Step:         2,
				Name:         name,
				InitialName:  initialName,
				IsEdit:       isEdit,
				Members:      members,
				Mode:         mode,
				Capabilities: caps,
				Error:        "Add at least one member to continue",
			}, true)
			return
		}
		h.renderCombosViewWithDraft(w, r, WizardDraft{
			Step:         3,
			Name:         name,
			InitialName:  initialName,
			IsEdit:       isEdit,
			Members:      members,
			Mode:         mode,
			Capabilities: caps,
		}, true)
		return

	case action == "back_step_2":
		h.renderCombosViewWithDraft(w, r, WizardDraft{
			Step:         2,
			Name:         name,
			InitialName:  initialName,
			IsEdit:       isEdit,
			Members:      members,
			Mode:         mode,
			Capabilities: caps,
		}, true)
		return

	case action == "next_step_3":
		selectedMode := r.FormValue("draft_mode")
		if selectedMode != "" {
			mode = selectedMode
		}
		if mode != "ordered" && mode != "pool" && mode != "fused" {
			mode = "ordered"
		}
		h.renderCombosViewWithDraft(w, r, WizardDraft{
			Step:         4,
			Name:         name,
			InitialName:  initialName,
			IsEdit:       isEdit,
			Members:      members,
			Mode:         mode,
			Capabilities: caps,
		}, true)
		return

	case action == "back_step_3":
		h.renderCombosViewWithDraft(w, r, WizardDraft{
			Step:         3,
			Name:         name,
			InitialName:  initialName,
			IsEdit:       isEdit,
			Members:      members,
			Mode:         mode,
			Capabilities: caps,
		}, true)
		return

	case action == "next_step_4":
		caps = r.Form["draft_caps"]
		h.renderCombosViewWithDraft(w, r, WizardDraft{
			Step:         5,
			Name:         name,
			InitialName:  initialName,
			IsEdit:       isEdit,
			Members:      members,
			Mode:         mode,
			Capabilities: caps,
		}, true)
		return

	case action == "back_step_4":
		h.renderCombosViewWithDraft(w, r, WizardDraft{
			Step:         4,
			Name:         name,
			InitialName:  initialName,
			IsEdit:       isEdit,
			Members:      members,
			Mode:         mode,
			Capabilities: caps,
		}, true)
		return

	case action == "submit_create":
		// Final save / create
		data, err := os.ReadFile(h.deps.Service.ConfigPath)
		if err != nil {
			http.Redirect(w, r, "/dashboard/combos?error="+url.QueryEscape("Failed to read configuration"), http.StatusSeeOther)
			return
		}
		rawTopo, err := config.ParseRawTopology(data)
		if err != nil {
			http.Redirect(w, r, "/dashboard/combos?error="+url.QueryEscape("Failed to parse configuration"), http.StatusSeeOther)
			return
		}

		if len(members) < 1 {
			http.Redirect(w, r, "/dashboard/combos?error="+url.QueryEscape("Combo must have at least one member"), http.StatusSeeOther)
			return
		}

		newCombo := config.Combo{
			Name:         name,
			Mode:         mode,
			Members:      members,
			Capabilities: caps,
		}

		if isEdit {
			found := false
			for i, c := range rawTopo.Combos {
				if c.Name == initialName {
					newCombo.Disabled = c.Disabled
					rawTopo.Combos[i] = newCombo
					found = true
					break
				}
			}
			if !found {
				rawTopo.Combos = append(rawTopo.Combos, newCombo)
			}
		} else {
			rawTopo.Combos = append(rawTopo.Combos, newCombo)
		}

		validTopo := config.Topology{
			Providers: rawTopo.Providers,
			Combos:    rawTopo.Combos,
		}
		if errs := config.ValidateTopology(validTopo, dialect.Names()); len(errs) > 0 {
			http.Redirect(w, r, "/dashboard/combos?error="+url.QueryEscape(fmt.Sprintf("Invalid topology: %v", errs[0])), http.StatusSeeOther)
			return
		}

		if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
			http.Redirect(w, r, "/dashboard/combos?error="+url.QueryEscape("Failed to write configuration"), http.StatusSeeOther)
			return
		}

		actionVerb := "created"
		if isEdit {
			actionVerb = "updated"
		}
		http.Redirect(w, r, "/dashboard/combos?flash="+url.QueryEscape(fmt.Sprintf("Successfully %s combo '%s'", actionVerb, name)), http.StatusSeeOther)
		return

	default:
		h.renderCombosViewWithDraft(w, r, WizardDraft{
			Step:         step,
			Name:         name,
			InitialName:  initialName,
			IsEdit:       isEdit,
			Members:      members,
			Mode:         mode,
			Capabilities: caps,
		}, true)
	}
}

func (h *DashboardHandler) handleCombosDelete(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/dashboard/combos?error="+url.QueryEscape("Invalid form submission"), http.StatusSeeOther)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Redirect(w, r, "/dashboard/combos?error="+url.QueryEscape("Combo name is required"), http.StatusSeeOther)
		return
	}

	data, err := os.ReadFile(h.deps.Service.ConfigPath)
	if err != nil {
		http.Redirect(w, r, "/dashboard/combos?error="+url.QueryEscape("Failed to read configuration"), http.StatusSeeOther)
		return
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		http.Redirect(w, r, "/dashboard/combos?error="+url.QueryEscape("Failed to parse configuration"), http.StatusSeeOther)
		return
	}

	idx := -1
	for i, c := range rawTopo.Combos {
		if c.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		http.Redirect(w, r, "/dashboard/combos?error="+url.QueryEscape(fmt.Sprintf("Combo '%s' not found", name)), http.StatusSeeOther)
		return
	}

	rawTopo.Combos = append(rawTopo.Combos[:idx], rawTopo.Combos[idx+1:]...)

	if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
		http.Redirect(w, r, "/dashboard/combos?error="+url.QueryEscape("Failed to write configuration"), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/dashboard/combos?flash="+url.QueryEscape(fmt.Sprintf("Successfully deleted combo '%s'", name)), http.StatusSeeOther)
}

func (h *DashboardHandler) handleCombosToggle(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/dashboard/combos?error="+url.QueryEscape("Invalid form submission"), http.StatusSeeOther)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Redirect(w, r, "/dashboard/combos?error="+url.QueryEscape("Combo name is required"), http.StatusSeeOther)
		return
	}

	data, err := os.ReadFile(h.deps.Service.ConfigPath)
	if err != nil {
		http.Redirect(w, r, "/dashboard/combos?error="+url.QueryEscape("Failed to read configuration"), http.StatusSeeOther)
		return
	}
	rawTopo, err := config.ParseRawTopology(data)
	if err != nil {
		http.Redirect(w, r, "/dashboard/combos?error="+url.QueryEscape("Failed to parse configuration"), http.StatusSeeOther)
		return
	}

	idx := -1
	for i, c := range rawTopo.Combos {
		if c.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		http.Redirect(w, r, "/dashboard/combos?error="+url.QueryEscape(fmt.Sprintf("Combo '%s' not found", name)), http.StatusSeeOther)
		return
	}

	rawTopo.Combos[idx].Disabled = !rawTopo.Combos[idx].Disabled

	if err := config.WriteTopology(h.deps.Service.ConfigPath, rawTopo); err != nil {
		http.Redirect(w, r, "/dashboard/combos?error="+url.QueryEscape("Failed to write configuration"), http.StatusSeeOther)
		return
	}

	verb := "enabled"
	if rawTopo.Combos[idx].Disabled {
		verb = "disabled"
	}
	http.Redirect(w, r, "/dashboard/combos?flash="+url.QueryEscape(fmt.Sprintf("Successfully %s combo '%s'", verb, name)), http.StatusSeeOther)
}
