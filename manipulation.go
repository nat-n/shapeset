package shapeset

import (
	"github.com/nat-n/geom"
	"github.com/nat-n/gomesh/cuboid"
	"github.com/nat-n/gomesh/mesh"
	"github.com/nat-n/gomesh/transformation"
	"math"
)

func (ss *ShapeSet) BoundingBox() (bb cuboid.Cuboid) {
	first := true
	ss.EachMesh(func(m mesh.MeshI) {
		if first {
			bb = *m.(*Mesh).BoundingBox
			first = false
		} else {
			bb = m.(*Mesh).BoundingBox.Union(bb)
		}
	})
	return
}

func (ss *ShapeSet) ScaleAndCenter(max_dimension float64) {
	bbox := ss.BoundingBox()
	center := bbox.Center()
	current_max_dim := math.Max(math.Max(bbox.Width(), bbox.Height()), bbox.Depth())
	scale_factor := max_dimension / current_max_dim

	// center then scale
	s := transformation.Scale(scale_factor)
	transform := s.Multiply(
		transformation.Translation(-center.GetX(), -center.GetY(), -center.GetZ()))

	all_vertices := make([]geom.Vec3I, 0)
	ss.EachMesh(func(m mesh.MeshI) {
		m.(*Mesh).Vertices.Each(func(v mesh.VertexI) {
			if !v.(*Vertex).IsShared() {
				all_vertices = append(all_vertices, geom.Vec3I(v))
			}
		})
	})

	ss.BordersIndex.Each(func(b *Border) {
		for _, v := range b.Vertices {
			all_vertices = append(all_vertices, geom.Vec3I(v))
		}
	})

	transform.ApplyToVec3(all_vertices...)
}
