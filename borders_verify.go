package shapeset

import (
	"errors"
	"github.com/nat-n/geom"
	"strconv"
)

// Border Verification:
// --------------------
// The VerifyBorders method checks that all the indexed borders in the ShapeSet
// actually match between meshes, i.e. that for any given vertex in any border
// in a given mesh, there a counterpart at the same point in the border and the
// at the same cartesian location, in every other mesh sharing that border.

// also checks that no vertex is part of multiple borders

// returns an error for the first violated expectation

type collatedVertex map[string]geom.Vec3

func (ss *ShapeSet) VerifyBorders() (err error) {
	// build up a list of border ids
	border_ids := make(map[string]int)
	for _, mw := range ss.Meshes {
		for border_id, border_indices := range mw.Borders {
			border_ids[border_id] = len(border_indices)
		}
	}

	// collate all border vertices from all meshes
	all_borders := make(map[string][]collatedVertex)
	for border_id, border_len := range border_ids {
		for mesh_name, m := range ss.Meshes {
			if border_indices, exists := m.Borders[border_id]; exists {
				if len(border_indices) != border_len {
					err = errors.New(
						"Border length mismatch in border " +
							border_id + ", found in mesh " + mesh_name)
					return
				}
				for i, vertex_index := range m.Borders[border_id] {
					if len(all_borders[border_id]) <= i {
						all_borders[border_id] = append(
							all_borders[border_id],
							make(map[string]geom.Vec3),
						)
					}
					vertex := ss.Meshes[mesh_name].Mesh.Verts.Get(vertex_index)[0]
					all_borders[border_id][i][mesh_name] = *vertex
				}
			}
		}
	}

	// Check all border verts exist at the same location across meshes
	for border_id, border_vertices := range all_borders {
		for i, border_vertex := range border_vertices {
			if !border_vertex.isValid() {
				err = errors.New(
					"Mismatching border vertex found in border " +
						border_id + ", at index " + strconv.Itoa(i))
				return
			}
		}
	}

	// Check that no vertex appears in multiple borders
	for _, mw := range ss.Meshes {
		// pivot the Borders object to map vertex indices onto border ids
		// there should only be one border id per vertex index.

		occurances := make(map[int][]string)
		for border_id, border_indices := range mw.Borders {
			for _, vert_index := range border_indices {
				occurances[vert_index] = append(
					occurances[vert_index],
					border_id,
				)
			}
		}

		for vert_index, border_ids := range occurances {
			if len(border_ids) > 1 {

				error_str := "Vertex " + strconv.Itoa(vert_index) + " of " +
					mw.Mesh.Name + " occurs in multiple borders : "
				for _, border_id1 := range border_ids {
					for border_desc, border_id2 := range ss.BordersIndex {
						if border_id1 == border_id2 {
							error_str += border_desc + " "
						}
					}
				}
				err = errors.New(error_str)
				return
			}
		}
	}

	return
}

func (cv *collatedVertex) isValid() bool {
	// check that the vertex location is the same across instances
	first := true
	var prev geom.Vec3
	for _, vertex := range *cv {
		if first {
			prev = vertex
		} else if vertex != prev {
			return false
		}
	}
	return true
}
