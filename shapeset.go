package shapeset

import (
	"github.com/nat-n/gomesh/cuboid"
	gomesh "github.com/nat-n/gomesh/mesh"
	"strconv"
	"strings"
)

type ShapeSet struct {
	Name         string
	Shapes       map[ShapeId]string
	Meshes       map[MeshId]*Mesh
	BordersIndex BorderIndex
}

type Mesh struct {
	gomesh.Mesh
	Borders     map[BorderId]*Border
	BoundingBox *cuboid.Cuboid
}

func (m *Mesh) Id() MeshId {
	return MeshIdFromString(m.Name)
}

func NewMesh(name string) *Mesh {
	return &Mesh{
		Mesh:    *gomesh.New(name),
		Borders: make(map[BorderId]*Border),
	}
}

func WrapMesh(m *gomesh.Mesh) *Mesh {
	m.ReindexVerticesAndFaces()
	// also wrap vertices; i.e. replaces mesh.Vertex with shapeset.vertex
	for i := 0; i < m.Vertices.Len(); i++ {
		if v, ok := m.Vertices.Get(i)[0].(*gomesh.Vertex); ok {
			m.Vertices.Update(i, &Vertex{Vertex: *v})
		}
	}

	// also wrap faces and update them to reference outer vertex object
	for i := 0; i < m.Faces.Len(); i++ {
		if fx, ok := m.Faces.Get(i)[0].(*gomesh.Face); ok {
			f := &Face{Face: *fx}
			f.SetA(m.Vertices.Get(f.GetA().GetLocationInMesh(m))[0])
			f.SetB(m.Vertices.Get(f.GetB().GetLocationInMesh(m))[0])
			f.SetC(m.Vertices.Get(f.GetC().GetLocationInMesh(m))[0])
			m.Faces.Update(i, f)
			f.GetA().AddFace(f)
			f.GetB().AddFace(f)
			f.GetC().AddFace(f)
		}
	}

	// iterate faces, build set of edges
	vert_pairs := make(map[gomesh.VertexPair]*Edge)
	m.Faces.EachWithIndex(func(i int, f gomesh.FaceI) {
		pairs := [3]gomesh.VertexPair{
			gomesh.MakeVertexPair(f.GetA(), f.GetB()),
			gomesh.MakeVertexPair(f.GetB(), f.GetC()),
			gomesh.MakeVertexPair(f.GetC(), f.GetA()),
		}
		for i, pair := range pairs {
			var e *Edge
			if e1, exists := vert_pairs[pair]; !exists {
				e = &Edge{VertexPair: pair}
				e.V1.(*Vertex).AddEdge(e)
				e.V2.(*Vertex).AddEdge(e)
				vert_pairs[pair] = e
			} else {
				e = e1
			}
			e.AddFace(f.(*Face))
			f.(*Face).Edges[i] = e
		}
	})

	return &Mesh{
		Mesh:        *m,
		Borders:     make(map[BorderId]*Border),
		BoundingBox: m.BoundingBox(),
	}
}

type ShapeId int
type MeshId [2]ShapeId

func ShapeIdFromString(s string) ShapeId {
	shape_num, e := strconv.Atoi(s)
	if e != nil {
		panic("Couldn't parse border id from: " + s)
	}
	return ShapeId(shape_num)
}

func (b *ShapeId) ToString() string {
	return strconv.Itoa(int(*b))
}

func MeshIdFromString(s string) MeshId {
	// ensures correct ordering
	parts := strings.Split(s, "-")
	shape_1, e1 := strconv.Atoi(parts[0])
	shape_2, e2 := strconv.Atoi(parts[1])
	if shape_1 > shape_2 {
		shape_1, shape_2 = shape_2, shape_1
	}
	if e1 != nil || e2 != nil {
		panic("Couldn't parse mesh id from: " + s)
	}
	return MeshId{ShapeId(shape_1), ShapeId(shape_2)}
}

func (m *MeshId) ToString() string {
	return strconv.Itoa(int(m[0])) + "-" + strconv.Itoa(int(m[1]))
}

func (m *MeshId) LessThan(n MeshId) bool {
	return m[0] < n[0] || m[0] == n[0] && m[1] < n[1]
}

func (m *MeshId) IncludesShape(s ShapeId) bool { return m[0] == s || m[1] == s }

type ByMeshIdPrecedence []MeshId

func (s ByMeshIdPrecedence) Len() int           { return len(s) }
func (s ByMeshIdPrecedence) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s ByMeshIdPrecedence) Less(i, j int) bool { return s[i].LessThan(s[j]) }

func (ss *ShapeSet) EachMesh(cb func(gomesh.MeshI)) {
	for _, m := range ss.Meshes {
		cb(m)
	}
}

func New(name string, labels map[string]string, meshes []*Mesh) (ss *ShapeSet) {
	ss = &ShapeSet{
		Name:   name,
		Shapes: make(map[ShapeId]string),
		Meshes: make(map[MeshId]*Mesh),
	}
	ss.ResetBorders()

	for shape_value_str, shape_label := range labels {
		shape_value, _ := strconv.Atoi(shape_value_str)
		ss.Shapes[ShapeId(shape_value)] = shape_label
	}

	for _, m := range meshes {
		ss.Meshes[m.Id()] = m
	}

	return ss
}
