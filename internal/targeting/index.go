package targeting

import (
	"sync"
)

type AgentLabels struct {
	AgentID   string
	Namespace string
	Labels    map[string]string
}

type Index struct {
	mu     sync.RWMutex
	agents map[string]AgentLabels
}

func NewIndex() *Index {
	return &Index{
		agents: make(map[string]AgentLabels),
	}
}

func (idx *Index) Upsert(agentID, namespace string, labels map[string]string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.agents[agentID] = AgentLabels{
		AgentID:   agentID,
		Namespace: namespace,
		Labels:    labels,
	}
}

func (idx *Index) Remove(agentID string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	delete(idx.agents, agentID)
}

func (idx *Index) Match(namespace string, sel Selector) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var matched []string
	for _, agent := range idx.agents {
		if agent.Namespace != namespace {
			continue
		}
		if sel.IsEmpty() || sel.Matches(agent.Labels) {
			matched = append(matched, agent.AgentID)
		}
	}
	return matched
}

func (idx *Index) All(namespace string) []AgentLabels {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var result []AgentLabels
	for _, agent := range idx.agents {
		if agent.Namespace == namespace {
			result = append(result, agent)
		}
	}
	return result
}