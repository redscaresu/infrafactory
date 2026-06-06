// topology_derive_genesys.go — S114-T6.
//
// Derives the connectivity + http_probe topology from fakegenesys's
// /mock/state output. Genesys is CCaaS, not IaaS: there are no VPCs,
// no LBs, no networking primitives. The "graph" is queue ↔ user
// membership, queue ↔ {skill, wrapupcode, language} references, and
// flow ↔ queue references. http_probe is always empty because Genesys
// doesn't expose externally-reachable URLs the way LBs do.
//
// The derived shape mirrors what deriveTopologyAWS / deriveTopologyScaleway
// return:
//
//	{
//	  "connectivity": { ... per-edge entries ... },
//	  "http_probe":   {}
//	}
//
// connectivity entries are keyed `routing_queue:{queueId}` and list
// the connected user_ids (members) + skill/wrapupcode/language ids.
package harness

import (
	"encoding/json"
	"fmt"
	"sort"
)

func deriveTopologyGenesys(stateJSON []byte) ([]byte, map[string]string, error) {
	var state struct {
		RoutingQueues       []json.RawMessage `json:"routing_queues"`
		RoutingQueueMembers []struct {
			QueueID string `json:"queueId"`
			UserID  string `json:"userId"`
		} `json:"routing_queue_members"`
		Flows []json.RawMessage `json:"flows"`
	}
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return nil, nil, fmt.Errorf("unmarshal genesys state: %w", err)
	}

	connectivity := map[string]map[string]any{}
	diagnostics := map[string]string{}

	// Build per-queue member lists from the queue_members grid.
	queueMembers := map[string][]string{}
	for _, m := range state.RoutingQueueMembers {
		queueMembers[m.QueueID] = append(queueMembers[m.QueueID], m.UserID)
	}

	for _, raw := range state.RoutingQueues {
		var q struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Skills []struct {
				ID string `json:"id"`
			} `json:"skills"`
			WrapupCodes []struct {
				ID string `json:"id"`
			} `json:"wrapupCodes"`
			Languages []struct {
				ID string `json:"id"`
			} `json:"languages"`
		}
		if err := json.Unmarshal(raw, &q); err != nil {
			continue
		}
		key := "routing_queue:" + q.ID
		members := queueMembers[q.ID]
		sort.Strings(members)
		skills := make([]string, 0, len(q.Skills))
		for _, s := range q.Skills {
			skills = append(skills, s.ID)
		}
		wrapups := make([]string, 0, len(q.WrapupCodes))
		for _, w := range q.WrapupCodes {
			wrapups = append(wrapups, w.ID)
		}
		languages := make([]string, 0, len(q.Languages))
		for _, l := range q.Languages {
			languages = append(languages, l.ID)
		}
		connectivity[key] = map[string]any{
			"name":         q.Name,
			"members":      members,
			"skills":       skills,
			"wrapup_codes": wrapups,
			"languages":    languages,
		}
		if len(members) == 0 && len(skills) == 0 {
			diagnostics[key] = "queue has no members and no skill requirements; calls may queue indefinitely"
		}
	}

	// Flow references — keyed `flow:{flowId}`. Only the lockedUser is
	// graph-bearing.
	for _, raw := range state.Flows {
		var f struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			State      string `json:"state"`
			LockedUser struct {
				ID string `json:"id"`
			} `json:"lockedUser"`
		}
		if err := json.Unmarshal(raw, &f); err != nil {
			continue
		}
		key := "flow:" + f.ID
		entry := map[string]any{
			"name":  f.Name,
			"state": f.State,
		}
		if f.LockedUser.ID != "" {
			entry["locked_by_user"] = f.LockedUser.ID
		}
		connectivity[key] = entry
	}

	body, err := json.Marshal(map[string]any{
		"connectivity": connectivity,
		"http_probe":   map[string]any{},
	})
	if err != nil {
		return nil, nil, err
	}
	return body, diagnostics, nil
}
