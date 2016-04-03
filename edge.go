package shapeset

import (
	"container/heap"
	"github.com/nat-n/geom"
	gomesh "github.com/nat-n/gomesh/mesh"
)

type Edge struct {
	gomesh.VertexPair
	Faces          []*Face
	Border         *Border
	CollapseTarget geom.Vec3
	Q              *geom.SymMat4
	Error          float64
	Collapsed      bool
	Protected      bool
	Removed        bool
}

func NewEdge(v1, v2 *Vertex) *Edge {
	a := gomesh.MakeVertexPair(v1, v2)
	return &Edge{
		VertexPair: a,
	}
}

func (e *Edge) Vertex1() *Vertex {
	return e.V1.(*Vertex)
}

func (e *Edge) Vertex2() *Vertex {
	return e.V2.(*Vertex)
}

func (e *Edge) HasBorder() bool {
	return e.Border != nil
}

func (e *Edge) ReferencesVertex(v *Vertex) bool {
	return e.Vertex1() == v || e.Vertex2() == v
}

func (e *Edge) ReferencesFace(f1 *Face) bool {
	for _, f2 := range e.Faces {
		if f1 == f2 {
			return true
		}
	}
	return false
}

func (e *Edge) EachFace(cb func(*Face)) {
	for _, f := range e.Faces {
		cb(f)
	}
}

func (e *Edge) AddFace(f *Face) {
	defer func() {
		assert("AddFace of Edge succeeded", func() bool {
			// e.Faces references f exactly once
			ref_count := 0
			for _, f_of_e := range e.Faces {
				if f_of_e == f {
					ref_count++
				}
			}
			return ref_count == 1
		})
	}()

	e.Faces = append(e.Faces, f)
}

func (e *Edge) RemoveFace(f *Face) {
	assert("RemoveFace of Edge called validly", func() bool {
		// e.Faces references f exactly once
		ref_count := 0
		for _, f_of_e := range e.Faces {
			if f_of_e == f {
				ref_count++
			}
		}
		return ref_count == 1
	})

	defer func() {
		assert("RemoveFace of Edge succeeded", func() bool {
			// e.Faces does not reference f
			for _, f_of_e := range e.Faces {
				if f_of_e == f {
					return false
				}
			}
			return true
		})
	}()

	for i, ef := range e.Faces {
		if f == ef {
			e.Faces = append(e.Faces[:i], e.Faces[i+1:]...)
			return
		}
	}
}

func (e *Edge) ReplaceVertex(old_vert, new_vert *Vertex) {
	defer func() {
		assert("ReplaceVertex of Edge succeeded", func() bool {
			return (
			// e no longer references old_vert
			e.V1 != old_vert && e.V2 != old_vert &&
				// e references new_vert exactly once
				((e.V1 == new_vert && e.V2 != new_vert) ||
					(e.V1 != new_vert && e.V2 == new_vert)))
		})
	}()
	if e.Vertex1() == old_vert {
		e.V1 = new_vert
	} else if e.Vertex2() == old_vert {
		e.V2 = new_vert
	} else {
		panic("Model assumption violated: Vertex old_vert must occur in ege e to be" +
			" replaced by vertex new_vert")
	}
}

// takes some other Edges between the same two vertices,
// copies over their Faces and disowns them
func (e *Edge) Merge(others ...*Edge) {
	for _, other_e := range others {
		assert("other_e is peer of e", func() bool {
			return e.Vertex1() == other_e.Vertex1() && e.Vertex2() == other_e.Vertex2() ||
				e.Vertex1() == other_e.Vertex2() && e.Vertex2() == other_e.Vertex1()
		})
		for _, f := range other_e.Faces {
			e.AddFace(f)
			f.ReplaceEdge(other_e, e)
		}
		other_e.Vertex1().RemoveEdge(other_e)
		other_e.Vertex2().RemoveEdge(other_e)
	}
}

type edgeHeap []*Edge

func (h edgeHeap) Len() int           { return len(h) }
func (h edgeHeap) Less(i, j int) bool { return h[i].Error < h[j].Error }
func (h edgeHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *edgeHeap) Push(x interface{}) {
	e := x.(*Edge)
	*h = append(*h, e)
}

func (h *edgeHeap) Replace(es []*Edge) {
	old := *h
	*h = old[:len(es)]
	copy(*h, es)
}

func (h *edgeHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// Find and call fix on each of the affected edges,
// this is inefficient, but not too bad, and I'm not sure how to avoid it
func (h *edgeHeap) UpdateEdges(affected_edges []*Edge) {
	possible_matches := make([]*Edge, len(affected_edges))
	copy(possible_matches, affected_edges)
	eh := *h
	for i, e := range eh {
		for j, possible_match := range possible_matches {
			if e == possible_match {
				heap.Fix(h, i)
				possible_matches = append(
					possible_matches[:j],
					possible_matches[j+1:]...,
				)
				break
			}
		}
	}
}

func (e *Edge) Collapse() (recalculated []*Edge) {
	// Collapsing an edge that shares a face with other border edges causes
	// complications that are easiest to just avoid.
	for _, f := range e.Faces {
		for _, e2 := range f.Edges {
			if e2 != e && e2.HasBorder() {
				return
			}
		}
	}

	eV1 := e.Vertex1()
	eV2 := e.Vertex2()

	// Collapsing an edge whose vertices don't belong to the same border, can
	// causes vertices V1 to gain faces from meshes that it was not part of
	// previously. A complication that it probably best avoided.
	if eV1.Border != eV2.Border {
		return
	}

	e.Collapsed = true
	eV2.CollapsedInto = eV1

	eV1.RemoveEdge(e)
	eV2.RemoveEdge(e)

	assert("e only references each of it's faces once", func() bool {
		for i := 0; i < len(e.Faces); i++ {
			for j := i + 1; j < len(e.Faces); j++ {
				if e.Faces[i] == e.Faces[j] {
					return false
				}
			}
		}
		return true
	})

	assert("no faces of e are already collapsed", func() bool {
		for _, f := range e.Faces {
			if f.Collapsed {
				return false
			}
		}
		return true
	})

	// collapse faces of e
	e.EachFace(func(f *Face) {
		// identify which other edges of this face connect to e.V1 and e.V2
		var v1e, v2e *Edge
		for _, f_edge := range f.Edges {
			if f_edge == e {
				continue
			} else if f_edge.ReferencesVertex(eV1) {
				v1e = f_edge
			} else if f_edge.ReferencesVertex(eV2) {
				v2e = f_edge
			} else {
				panic("Model assumption violated: Either vertex eV1 or eV2 of edge e " +
					"must be referenced by edge f_edge of face f that references e")
			}
		}
		if v1e == nil || v2e == nil {
			panic("Model assumption violated: Both vertices eV1 and eV2 of edge e " +
				"must be referenced by another edge e2 of face f references by e, where" +
				" e != e2")
		}

		// remove all references to this face
		f.Collapsed = true
		v1e.RemoveFace(f)
		v2e.RemoveFace(f)
		f.EachVertex(func(v gomesh.VertexI) { v.(*Vertex).RemoveFace(f) })

		// remove all references to the v2 Edge of this face
		v2e.Collapsed = true
		v2e.EachFace(func(v2ef *Face) {
			// move faces of v2e to v1e (except f)
			if v2ef == f {
				return
			}
			v2ef.ReplaceEdge(v2e, v1e)
			v1e.AddFace(v2ef)
		})
		v2e.Faces = v2e.Faces[:0]
		v2e.Vertex1().RemoveEdge(v2e)
		v2e.Vertex2().RemoveEdge(v2e)
	})
	e.Faces = e.Faces[:0]

	// transfer remaing edges and faces of eV2 to eV1
	eV2.EachEdge(func(v2e *Edge) {
		if !v2e.Collapsed {
			v2e.ReplaceVertex(eV2, eV1)
			eV1.AddEdge(v2e)
		}
	})
	eV2.EachFace(func(v2fi gomesh.FaceI) {
		v2f := v2fi.(*Face)
		if !v2f.Collapsed {
			v2f.ReplaceVertex(eV2, eV1)
			eV1.AddFace(v2f)
		}
	})

	// Update position of e.V1 to e.CollapseTarget
	e.V1.SetX(e.CollapseTarget.X)
	e.V1.SetY(e.CollapseTarget.Y)
	e.V1.SetZ(e.CollapseTarget.Z)

	// Recalculate error quarics for v1 and affected edges
	// this will amplify (double count) the planes of the collapsed faces!
	e.Vertex1().Q.Add(e.Vertex2().Q)
	for _, v1e := range e.Vertex1().Edges {
		if v1e.HasBorder() {
			v1e.calculateError()
		}
		recalculated = append(recalculated, v1e)
	}

	e.Border.RemoveEdge(e)
	eV2.Border.RemoveVertex(eV2)

	return
}

func (e *Edge) calculateError() {
	// Calculate error quadric for this Edge as sum of vertex error quadrics
	Q := geom.SymMat4{}
	e.Q = &Q

	e.Q.Add(e.Vertex1().Q)
	e.Q.Add(e.Vertex2().Q)

	// Optimised determinant formula when the bottom row of Q is subsituted for [0, 0, 0, 1]
	det := -Q[2]*Q[4]*Q[2] + Q[1]*Q[5]*Q[2] + Q[2]*Q[1]*Q[5] - Q[0]*Q[5]*Q[5] - Q[1]*Q[1]*Q[7] + Q[0]*Q[4]*Q[7]
	if det != 0 {
		// Optimised implementation of Q^-1 * {{0},{0},{0},{1}}
		e.CollapseTarget.X = (Q[3]*Q[5]*Q[5] - Q[2]*Q[6]*Q[5] - Q[3]*Q[4]*Q[7] + Q[1]*Q[6]*Q[7] + Q[2]*Q[4]*Q[8] - Q[1]*Q[5]*Q[8]) / det
		e.CollapseTarget.Y = (Q[2]*Q[6]*Q[2] - Q[3]*Q[5]*Q[2] + Q[3]*Q[1]*Q[7] - Q[0]*Q[6]*Q[7] - Q[2]*Q[1]*Q[8] + Q[0]*Q[5]*Q[8]) / det
		e.CollapseTarget.Z = (Q[3]*Q[4]*Q[2] - Q[1]*Q[6]*Q[2] - Q[3]*Q[1]*Q[5] + Q[0]*Q[6]*Q[5] + Q[1]*Q[1]*Q[8] - Q[0]*Q[4]*Q[8]) / det
	} else {

		// Determine which is best, V1, V2 or their midpoint
		midpoint := e.V1.Mean(e.V2)
		v1_error := Q.VertexError(e.V1.Clone())
		v2_error := Q.VertexError(e.V2.Clone())
		midpoint_error := Q.VertexError(midpoint)

		if v1_error < v2_error {
			if v1_error < midpoint_error {
				e.CollapseTarget = e.V1.Clone()
			} else {
				e.CollapseTarget = midpoint
			}
		} else {
			if v2_error < midpoint_error {
				e.CollapseTarget = e.V2.Clone()
			} else {
				e.CollapseTarget = midpoint
			}
		}
	}

	e.Error = Q.VertexError(e.CollapseTarget)
}
