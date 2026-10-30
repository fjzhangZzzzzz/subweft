package compile

import (
	"encoding/json"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
)

// inputDigest distinguishes human edits from fetched content. Only dependencies
// used by this result participate; unrelated library edits cannot block updates.
// The digest stays internal and contains no recoverable credentials.
func inputDigest(p domain.Profile, snap domain.Snapshot, result Result) string {
	inputs := map[string]any{}
	for _, source := range snap.Sources {
		if _, used := result.Sources[source.ID]; !used {
			continue
		}
		source.Revision = 0
		source.Nodes = nil
		source.LastSuccess, source.LastAttempt = time.Time{}, time.Time{}
		source.LastError = ""
		inputs["source/"+source.ID] = source
	}
	for _, node := range snap.Nodes {
		for _, group := range p.Groups {
			for _, id := range group.NodeIDs {
				if id == node.ID {
					node.Revision = 0
					inputs["node/"+id] = node
				}
			}
		}
	}
	for _, resource := range snap.Resources {
		if _, used := result.Resources[resource.ID]; !used {
			continue
		}
		resource.Revision = 0
		resource.LastSuccess, resource.LastAttempt = time.Time{}, time.Time{}
		resource.LastError = ""
		resource.Count = 0
		if resource.Kind == "http" {
			resource.Digest = ""
		}
		inputs["resource/"+resource.ID] = resource
	}
	data, _ := json.Marshal(inputs)
	return Digest(data)
}
