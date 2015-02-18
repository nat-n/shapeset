package shapeset

import (
	"github.com/nat-n/gomesh/cuboid"
	"github.com/nat-n/gomesh/mesh"
	"strconv"
)

type ShapeSet struct {
	Name         string
	BordersIndex map[string]string
	Meshes       map[string]MeshWrapper
	Shapes       map[int]string
}

type MeshWrapper struct {
	Mesh        *mesh.Mesh
	Borders     map[string][]int
	BoundingBox *cuboid.Cuboid
}

func New(name string, meshes []*mesh.Mesh, labels map[string]string) (ss *ShapeSet) {
	ss = &ShapeSet{}

	ss.Name = name

	ss.Shapes = make(map[int]string)
	for shape_value_str, shape_label := range labels {
		shape_value, _ := strconv.Atoi(shape_value_str)
		ss.Shapes[shape_value] = shape_label
	}

	ss.Meshes = make(map[string]MeshWrapper)
	for _, m := range meshes {
		ss.Meshes[m.Name] = MeshWrapper{m, make(map[string][]int), m.BoundingBox()}
	}

	return ss
}
