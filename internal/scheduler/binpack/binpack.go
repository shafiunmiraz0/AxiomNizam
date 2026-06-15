// Package binpack implements first-fit-decreasing (FFD) and
// best-fit-decreasing (BFD) bin-packing allocators for workload
// placement.  They are the scoring primitives behind Nomad's
// bin-packing scheduler: given a set of nodes with residual
// capacity and a set of allocations needing placement, return an
// assignment that packs allocations densely (FFD) or evenly
// (best-fit on the most-loaded-that-still-fits node).
//
// The allocators are pure functions — they mutate neither inputs
// nor persistent state — so they compose cleanly inside tenant
// simulators and scheduling preview endpoints.
package binpack

import (
	"sort"

	"axiomnizam.bitbd.net/axiomnizam/internal/planner"
)

// Node is what the allocator operates on.
type Node struct {
	// ID identifies the node.
	ID string
	// Available is the remaining free capacity.
	Available planner.Resources
	// Weight is a multiplier applied to score — higher weights pull
	// placements toward preferred nodes (e.g. same-region).
	Weight float64
}

// Request names what to place.
type Request struct {
	// ID is the allocation ID.
	ID string
	// Resources is the demand.
	Resources planner.Resources
}

// Assignment binds one Request to one Node, or records a failure.
type Assignment struct {
	// RequestID echoes Request.ID.
	RequestID string
	// NodeID is set when the request was placed.
	NodeID string
	// Placed is false when no node in the input set could host the
	// request; callers should treat this as a signal to add capacity
	// or relax constraints.
	Placed bool
}

// Strategy selects the packing heuristic.
type Strategy int

const (
	// StrategyFirstFit places each request on the first node with
	// enough room.  Fast, deterministic, produces denser packing
	// than BestFit when requests vary in size.
	StrategyFirstFit Strategy = iota
	// StrategyBestFit chooses the node with the least leftover
	// capacity after the placement — packs densely but risks
	// fragmentation.
	StrategyBestFit
	// StrategyWorstFit chooses the node with the most leftover —
	// spreads allocations evenly.  Good for latency-sensitive
	// workloads where head-room matters.
	StrategyWorstFit
)

// Allocate runs the chosen strategy.  Requests are sorted
// size-descending ("Decreasing" in FFD / BFD) before placement so
// that the largest allocations lock in their preferred node before
// smaller ones crowd them out.
func Allocate(nodes []Node, requests []Request, strategy Strategy) []Assignment {
	// Copy inputs so the caller's slices are untouched.
	ns := make([]Node, len(nodes))
	copy(ns, nodes)
	rs := make([]Request, len(requests))
	copy(rs, requests)

	// Decreasing-size sort (sum of CPU+Mem+Disk as a crude proxy for
	// "size"; real schedulers would use a weighted norm).
	sort.Slice(rs, func(i, j int) bool {
		return score(rs[i].Resources) > score(rs[j].Resources)
	})

	out := make([]Assignment, 0, len(rs))
	for _, req := range rs {
		idx := pick(ns, req.Resources, strategy)
		if idx < 0 {
			out = append(out, Assignment{RequestID: req.ID, Placed: false})
			continue
		}
		ns[idx].Available = ns[idx].Available.Sub(req.Resources)
		out = append(out, Assignment{
			RequestID: req.ID,
			NodeID:    ns[idx].ID,
			Placed:    true,
		})
	}
	return out
}

// score is the sort key for size-descending.
func score(r planner.Resources) int64 { return r.CPU + r.Memory + r.Disk }

// pick returns the index of the chosen node, or -1 if none fit.
func pick(nodes []Node, need planner.Resources, strategy Strategy) int {
	best := -1
	var bestLeft int64
	switch strategy {
	case StrategyFirstFit:
		for i := range nodes {
			if nodes[i].Available.Fits(need) {
				return i
			}
		}
		return -1
	case StrategyBestFit:
		for i := range nodes {
			if !nodes[i].Available.Fits(need) {
				continue
			}
			left := score(nodes[i].Available.Sub(need))
			if best < 0 || left < bestLeft {
				best = i
				bestLeft = left
			}
		}
	case StrategyWorstFit:
		for i := range nodes {
			if !nodes[i].Available.Fits(need) {
				continue
			}
			left := score(nodes[i].Available.Sub(need))
			if best < 0 || left > bestLeft {
				best = i
				bestLeft = left
			}
		}
	}
	return best
}
