package shapeset

import (
	"github.com/nat-n/geom"
	"github.com/nat-n/gomesh/mesh"
	"strconv"
	"strings"
)

// Compose a mesh of the surface of the region defined by the shapes indexed by
// the given values.
func (ss *ShapeSet) ComposeRegion(shape_ids ...int) (result mesh.Mesh, err error) {
	// initialise the output mesh with an appropriate name
	shape_names := make([]string, 0, len(shape_ids))
	for _, shape_id := range shape_ids {
		shape_names = append(shape_names, ss.Shapes[ShapeId(shape_id)])
	}
	new_mesh_name := strings.Join(shape_names, "_")
	result = *mesh.New(new_mesh_name)

	// Collect meshes, and determine whether each mesh's normals will need inverting
	meshes := make(map[*Mesh]bool)
	for _, m := range ss.Meshes {
		shape_id_strs := strings.Split(m.GetName(), "-")
		shape_id_1, _ := strconv.ParseInt(shape_id_strs[0], 10, 64)
		shape_id_2, _ := strconv.ParseInt(shape_id_strs[1], 10, 64)
		// must invert if only front shape of mesh fragment is included in region
		must_invert := intInSlice(int(shape_id_1), shape_ids)
		if must_invert != intInSlice(int(shape_id_2), shape_ids) {
			meshes[m] = must_invert
		}
	}

	// tracks which borders have already had at least one mesh included
	result_verts := make(map[geom.Vec3]*Vertex)
	for m, must_invert := range meshes {
		m.Faces.Each(func(f mesh.FaceI) {
			f2 := &Face{Face: mesh.Face{Vertices: [3]mesh.VertexI{}}}
			i := 0
			f.EachVertex(func(vi mesh.VertexI) {
				v3 := vi.(*Vertex).Vec3
				vert, encountered := result_verts[v3]
				if !encountered {
					vert = &Vertex{
						Vertex: mesh.Vertex{
							Vec3:   v3,
							Meshes: make(map[mesh.Mesh]int),
						},
					}
					result.Vertices.Append(vert)
					vert.SetLocationInMesh(&result, result.Vertices.Len()-1)
					result_verts[v3] = vert
				}
				if must_invert {
					// swap first and second vertices to invert the face
					if i == 0 {
						f2.Vertices[1] = vert
					} else if i == 1 {
						f2.Vertices[0] = vert
					} else {
						f2.Vertices[2] = vert
					}
				} else {
					f2.Vertices[i] = vert
				}
				vert.AddFace(f2)
				i++
			})
			result.Faces.Append(f2)
		})
	}

	return
}
