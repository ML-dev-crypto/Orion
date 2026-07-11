package cluster

import (
	"fmt"
	"strings"
)

// Config is everything the cluster package needs to construct a
// Cluster. Deliberately does NOT include Port — that's an HTTP server
// concern (which address to bind to), not a cluster membership/
// leadership concern, so it has no business living here. cmd/node
// tracks Port itself, separately, in its own local config type.
type Config struct {
	NodeID string
	Peers  []Node // includes every node in the cluster, INCLUDING self
}

// ParsePeers parses "id=address,id=address,..." into a []Node. Pure
// string parsing, no I/O. Rejects duplicate IDs and duplicate addresses
// — both represent misconfiguration that would cause silent misbehaviour
// later (split-brain on duplicate IDs, ambiguous routing on duplicate
// addresses).
func ParsePeers(s string) ([]Node, error) {
	parts := strings.Split(s, ",")
	nodes := make([]Node, 0, len(parts))
	seenIDs := make(map[string]struct{})
	seenAddrs := make(map[string]struct{})
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idAddr := strings.SplitN(part, "=", 2)
		if len(idAddr) != 2 || idAddr[0] == "" || idAddr[1] == "" {
			return nil, fmt.Errorf("cluster: invalid peer entry %q, expected format id=address", part)
		}
		id, addr := idAddr[0], idAddr[1]
		if _, exists := seenIDs[id]; exists {
			return nil, fmt.Errorf("cluster: duplicate peer id %q", id)
		}
		seenIDs[id] = struct{}{}
		if _, exists := seenAddrs[addr]; exists {
			return nil, fmt.Errorf("cluster: duplicate peer address %q", addr)
		}
		seenAddrs[addr] = struct{}{}
		nodes = append(nodes, Node{ID: id, Address: addr})
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("cluster: peer list produced zero valid entries")
	}
	return nodes, nil
}

// BootstrapLeader deterministically picks a leader from a set of nodes
// by comparing IDs lexicographically and returning the highest.
//
// The name is deliberate: this is scaffolding for the phase before
// real election exists, not a permanent algorithm. A later stage
// replaces the CALL SITE with the outcome of a real vote — but the
// tie-breaking rule itself (highest ID wins) is worth reusing there
// too, rather than inventing a second one.
func BootstrapLeader(nodes []Node) string {
	if len(nodes) == 0 {
		return ""
	}
	highest := nodes[0].ID
	for _, n := range nodes[1:] {
		if n.ID > highest {
			highest = n.ID
		}
	}
	return highest
}
