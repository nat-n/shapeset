package shapeset

import (
	"github.com/nat-n/geom"
	"github.com/nat-n/gomesh/mesh"
)

type Face struct {
	mesh.Face
	Edges     [3]*Edge
	Collapsed bool
	Kp        *geom.SymMat4
}

// Calculate the Kp fundemental error matrix of a Face, i.e. quadric of plane
func (f *Face) calculateKp() *geom.SymMat4 {
	// get corner vertices as vectors
	v1 := f.Vertices[0]
	v2 := f.Vertices[1]
	v3 := f.Vertices[2]

	// calculate center point of Face
	center := v1.Mean(v2, v3)

	// calcualte Face normal
	t := f.AsTriangle()
	n := t.Normal()

	// use abcd variable names like in the standard explanations
	a, b, c := n.X, n.Y, n.Z
	d := -(a*center.X + b*center.Y + c*center.Z)

	return &geom.SymMat4{
		a * a, a * b, a * c, a * d,
		b * b, b * c, b * d,
		c * c, c * d,
		d * d,
	}
}

func (f *Face) ReferencesEdge(e *Edge) bool {
	return f.Edges[0] == e || f.Edges[1] == e || f.Edges[2] == e
}

func (f *Face) ReplaceEdge(old_edge, new_edge *Edge) {
	defer func() {
		assert("ReplaceEdge of Face succeeded", func() bool {
			return (
			// f.Edges no longer references old_edge
			f.Edges[0] != old_edge && f.Edges[1] != old_edge && f.Edges[2] != old_edge &&
				// f.Vertices references new_vert exactly once
				((f.Edges[0] == new_edge && f.Edges[1] != new_edge && f.Edges[2] != new_edge) ||
					(f.Edges[0] != new_edge && f.Edges[1] == new_edge && f.Edges[2] != new_edge) ||
					(f.Edges[0] != new_edge && f.Edges[1] != new_edge && f.Edges[2] == new_edge)))
		})
	}()

	if f.Edges[0] == old_edge {
		f.Edges[0] = new_edge
	} else if f.Edges[1] == old_edge {
		f.Edges[1] = new_edge
	} else if f.Edges[2] == old_edge {
		f.Edges[2] = new_edge
	} else {
		panic("didn't find old_edge to replace")
	}
}
