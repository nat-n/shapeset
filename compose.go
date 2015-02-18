package shapeset

import (
	"github.com/nat-n/gomesh/mesh"
	"github.com/nat-n/gomesh/triplebuffer"
	"strconv"
	"strings"
)

// Compose a mesh of the surface of the region defined by the shapes indexed by
// the given values.
func (ss *ShapeSet) ComposeRegion(shape_ids ...int) (m mesh.Mesh, err error) {
	// initialise the output mesh with an appropriate name
	shape_names := make([]string, 0, len(shape_ids))
	for _, shape_id := range shape_ids {
		shape_names = append(shape_names, ss.Shapes[shape_id])
	}
	new_mesh_name := strings.Join(shape_names, "_")
	m = *mesh.New(new_mesh_name)

	// Collect meshes, and determine whether the mesh's normals will need inverting
	meshes := make(map[string]bool)
	for _, mw := range ss.Meshes {
		shape_id_strs := strings.Split(mw.Mesh.Name, "-")
		shape_id_1, _ := strconv.ParseInt(shape_id_strs[0], 10, 64)
		shape_id_2, _ := strconv.ParseInt(shape_id_strs[1], 10, 64)
		// must invert if front shape of mesh fragment is in included in region
		must_invert := intInSlice(int(shape_id_1), shape_ids)
		if must_invert != intInSlice(int(shape_id_2), shape_ids) {
			meshes[mw.Mesh.Name] = must_invert
		}
	}

	// Determine the buffer sizes required by the new mesh
	total_vertices := 0
	total_faces := 0
	for _, mw := range ss.Meshes {
		total_vertices += mw.Mesh.Verts.Len()
		total_faces += mw.Mesh.Faces.Len()
	}
	vertices_to_skip := 0
	for border_id, border_desc := range ss.BordersIndex {
		bordering_meshes := strings.Split(border_desc, "_")
		participating_meshes := make([]string, 0)
		for mesh_name, _ := range meshes {
			mw := ss.Meshes[mesh_name]
			if stringInSlice(mw.Mesh.Name, bordering_meshes) {
				participating_meshes = append(participating_meshes, mw.Mesh.Name)
			}
		}
		border_length := len(ss.Meshes[bordering_meshes[0]].Borders[border_id])
		for i := 0; i < len(participating_meshes)-1; i++ {
			vertices_to_skip += border_length
		}
	}

	// Initialise empty buffers in m with exactly the required length array
	//  underlying
	m.Verts = triplebuffer.NewVertexBuffer()
	m.Verts.Buffer = make([]float64, 0, (total_vertices-vertices_to_skip)*3)
	m.Norms.Buffer = make([]float64, 0, (total_vertices-vertices_to_skip)*3)
	m.Faces.Buffer = make([]int, 0, (total_faces)*3)

	combined_borders := make(map[string][]int)

	// Build up buffers with vertex remapping
	for mesh_name, _ := range meshes {
		index_map := make(map[int]int)
		mw := ss.Meshes[mesh_name]
		// Identify borders shared between mw.Borders and combined_borders
		shared_borders := make([]string, 0)
		for border_id, _ := range mw.Borders {
			if _, is_shared := combined_borders[border_id]; is_shared {
				shared_borders = append(shared_borders, border_id)
			}
		}
		// map shared border vertex indices onto indices of counterparts in new mesh
		for _, shared_border := range shared_borders {
			for i, index := range mw.Borders[shared_border] {
				index_map[index] = combined_borders[shared_border][i]
			}
		}

		// Copy over vertices and vertex normals, maintain index_map accordingly
		if meshes[mw.Mesh.Name] { // must invert this mesh's normals
			mw.Mesh.Verts.EachWithIndex(func(i int, x, y, z float64) {
				if _, already_indexed := index_map[i]; !already_indexed {
					index_map[i] = m.Verts.Len()
					m.Verts.Append(x, y, z)
					if mw.Mesh.Norms.Len() > 0 {
						next_normal := mw.Mesh.Norms.Get(i)[0]
						m.Norms.Append(-next_normal.X, -next_normal.Y, -next_normal.Z)
					}
				}
			})
		} else {
			mw.Mesh.Verts.EachWithIndex(func(i int, x, y, z float64) {
				if _, already_indexed := index_map[i]; !already_indexed {
					index_map[i] = m.Verts.Len()
					m.Verts.Append(x, y, z)
					if mw.Mesh.Norms.Len() > 0 {
						v := mw.Mesh.Norms.Get(i)[0]
						m.Norms.Append(v.X, v.Y, v.Z)
					}
				}
			})
		}

		// Copy over faces, remapping each vertex
		if meshes[mw.Mesh.Name] { // must invert this mesh's normals
			mw.Mesh.Faces.Each(func(x, y, z int) {
				m.Faces.Append(index_map[x], index_map[z], index_map[y])
			})
		} else {
			mw.Mesh.Faces.Each(func(x, y, z int) {
				m.Faces.Append(index_map[x], index_map[y], index_map[z])
			})
		}

		// Merge mw.Mesh.Borders into combined_borders
		for border_id, mw_border := range mw.Borders {
			combined_borders[border_id] = make([]int, len(mw_border))
			for i, border_vertex_index := range mw_border {
				combined_borders[border_id][i] = index_map[border_vertex_index]
			}
		}
	}

	return
}
