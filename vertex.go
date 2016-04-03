package shapeset

import (
	"github.com/nat-n/geom"
	"github.com/nat-n/gomesh/mesh"
)

type Vertex struct {
	mesh.Vertex
	Q             *geom.SymMat4
	Edges         []*Edge
	Border        *Border
	CollapsedInto *Vertex
}

func (v *Vertex) IsShared() bool {
	return v.Border != nil
}

func (v *Vertex) ReferencesEdge(e1 *Edge) bool {
	for _, e2 := range v.Edges {
		if e1 == e2 {
			return true
		}
	}
	return false
}

func (v *Vertex) EachEdge(cb func(*Edge)) {
	for _, e := range v.Edges {
		cb(e)
	}
}

func (v *Vertex) AddEdge(e *Edge) {
	defer func() {
		assert("AddEdge of Vertex succeeded", func() bool {
			// v.Edges references e exactly once
			ref_count := 0
			for _, e_of_v := range v.Edges {
				if e_of_v == e {
					ref_count++
				}
			}
			return ref_count == 1
		})
	}()

	v.Edges = append(v.Edges, e)
}

func (v *Vertex) RemoveEdge(e *Edge) {
	assert("RemoveEdge of Vertex called validly", func() bool {
		// v.Edges references e exactly once
		ref_count := 0
		for _, e_of_v := range v.Edges {
			if e_of_v == e {
				ref_count++
			}
		}
		return ref_count == 1
	})

	defer func() {
		assert("RemoveEdge of Vertex succeeded", func() bool {
			// v.Edges does not reference e
			for _, e_of_v := range v.Edges {
				if e_of_v == e {
					return false
				}
			}
			return true
		})
	}()

	for i, e2 := range v.Edges {
		if e == e2 {
			v.Edges = append(v.Edges[:i], v.Edges[i+1:]...)
			return
		}
	}
}

func (v *Vertex) calculateError() {
	// Calculate error quadric for this Vertex as sum of face error quadrics
	Q := geom.SymMat4{}
	v.Q = &Q
	v.EachFace(func(f mesh.FaceI) {
		v.Q.Add(f.(*Face).Kp)
	})
}
