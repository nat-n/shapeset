package shapeset

import (
	"errors"
	"github.com/nat-n/gomesh/cuboid"
	"github.com/nat-n/gomesh/mesh"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Goal:
// -----
// To create populate the Borders of each MeshWapper in the ShapeSet.
// The Borders map indexes the border vertices of the associated mesh by
// border identifer such that each indexed vertex in value of Borders, matches
// the corresponding vertex in other meshes that have the same border.
// This allows two meshes that share a border to be efficiently joined together
// along their border, by merging the vertices each of them references of the
// shared border.

// Strategy:
// ------------------
// Make a map with the boundaries for each mesh,
//   and the bounding boxes for each boundary
// For each pair of meshes (m1, m2) with intersecting bounding boxes:
//   For each pair of boundaries (m1.b, m2.b) with intersecting bounding boxes:
//     "We need to find which vertices from m1.b match which vertices from m2.b"
//     Created a collection with all vertices from both borders and sort by;
//       x, y, z values, then which mesh (0 or 1) they came from, so that we
//       can then find pairs of vertices where the first is from m1, and the
//       second from m2 and their and x,y,z values are identical by iterating
//       over the resulting sorted collection.

type boundaryDetails struct {
	Verts       *[]*vertexWrapper
	BoundingBox cuboid.Cuboid
}

type boundarySet struct {
	Boundaries []*boundaryDetails
	MeshName   string
}

type vertexWrapper struct {
	Vertex   *[3]float64
	Mesh     int // 0 for mesh1 or 1 for mesh2
	MeshName string
	Index    int // index of this vertex in the m1.Verts or m2.Verts
}

type sortableVertices []*vertexWrapper

func (vs sortableVertices) Len() int      { return len(vs) }
func (vs sortableVertices) Swap(i, j int) { vs[i], vs[j] = vs[j], vs[i] }

type vertsByPosition struct{ sortableVertices }

func (vs vertsByPosition) Less(i, j int) bool {
	a := vs.sortableVertices[i]
	b := vs.sortableVertices[j]
	if a.Vertex[0] > b.Vertex[0] {
		return false
	} else if a.Vertex[0] == b.Vertex[0] {
		if a.Vertex[1] > b.Vertex[1] {
			return false
		} else if a.Vertex[1] == b.Vertex[1] {
			if a.Vertex[2] > b.Vertex[2] {
				return false
			} else if a.Vertex[2] == b.Vertex[2] {
				if a.Index > b.Index {
					return false
				}
			}
		}
	}
	return true
}

type vertsByMesh struct{ sortableVertices }

func (vs vertsByMesh) Less(i, j int) bool {
	return vs.sortableVertices[i].MeshName < vs.sortableVertices[j].MeshName
}

type newBorderVertex struct {
	MeshName    string
	BorderId    string
	VertexIndex int
}

func (ss *ShapeSet) IndexBorders() (err error) {

	// Clear existing borders
	ss.BordersIndex = make(map[string]string)
	for _, wrapped_mesh := range ss.Meshes {
		// for some reason simply reassigning wrapped_mesh.Borders doesn't work!
		for border_id, _ := range wrapped_mesh.Borders {
			delete(wrapped_mesh.Borders, border_id)
		}
	}

	boundaries := make(map[string][]*boundaryDetails)

	var wg sync.WaitGroup

	// Compose a map with the boundaries with bounding boxes for each mesh
	new_boundaries := make(chan *boundarySet, 16)
	for _, m := range ss.Meshes {
		wg.Add(1)
		go func(mesh1 *mesh.Mesh) {
			m_boundaries := mesh1.IdentifyBoundaries()
			mesh1_boundaries := make([]*boundaryDetails, len(m_boundaries))
			for i, boundary := range m_boundaries {
				wrapped_boundary := make([]*vertexWrapper, len(boundary))
				for j, bv := range boundary {
					vert := [3]float64{}
					copy(vert[:], mesh1.Verts.Get(bv))
					wrapped_boundary[j] = &vertexWrapper{
						Vertex:   &vert,
						MeshName: mesh1.Name,
						Index:    bv,
					}
				}
				mesh1_boundaries[i] = &boundaryDetails{
					&wrapped_boundary,
					*mesh1.SubsetBoundingBox(boundary),
				}
			}
			new_boundaries <- &boundarySet{mesh1_boundaries, mesh1.Name}
		}(m.Mesh)
	}

	// Recieve new_boundaries until they're all done
	go func() {
		wg.Wait()
		close(new_boundaries)
	}()
	for new_boundary_set := range new_boundaries {
		boundaries[new_boundary_set.MeshName] = new_boundary_set.Boundaries
		wg.Done()
	}

	// The following block compares all borders to build up the vertexOccurances
	//  map.
	vertexOccurances := make(map[[3]float64][]*vertexWrapper)
	new_vertex_occurances := make(chan map[[3]float64][]*vertexWrapper, 16)
	for mesh_name1, m1 := range ss.Meshes {
		for mesh_name2, m2 := range ss.Meshes {
			if mesh_name1 >= mesh_name2 ||
				!m1.BoundingBox.Expanded(0.01).Intersects(
					m2.BoundingBox.Expanded(0.01)) {
				// Make sure we only deal with each pair of meshes once,
				//  and ignore pairs of meshes whose bounding boxes dont intersect
				continue
			}
			wg.Add(1)
			go func(mesh_name1, mesh_name2 string, boundaries1, boundaries2 []*boundaryDetails) {
				newVertexOccurances := make(map[[3]float64][]*vertexWrapper)
				for _, boundary1 := range boundaries1 {
					for _, boundary2 := range boundaries2 {
						if !boundary1.BoundingBox.Expanded(0.01).Intersects(
							boundary2.BoundingBox.Expanded(0.01)) {
							continue
						}
						// Now we need to identify any matching vertices between boundary1
						// and boundary2.
						// This is done using a sort wrapper to order the verts so that
						// potentially
						b1_len := len(*boundary1.Verts)
						b2_len := len(*boundary2.Verts)
						verts := make(
							[]*vertexWrapper,
							(b1_len + b2_len),
						)
						// Loop over the vertices in both borders to fill in verts
						for i, wrapped_vert := range *boundary1.Verts {
							verts[i] = &vertexWrapper{
								Vertex:   wrapped_vert.Vertex,
								Mesh:     0,
								MeshName: mesh_name1,
								Index:    wrapped_vert.Index,
							}
						}
						for i, wrapped_vert := range *boundary2.Verts {
							verts[b1_len+i] = &vertexWrapper{
								Vertex:   wrapped_vert.Vertex,
								Mesh:     1,
								MeshName: mesh_name2,
								Index:    wrapped_vert.Index,
							}
						}

						// Sort verts so that any border vertex from m2 that is potentially
						// collocated with a border vertex from m1 follows it directly in
						// the array.
						sort.Sort(vertsByPosition{verts})

						// iterate over the sorted verts and identify pairs of verts from
						// different meshes at the same location
						prev_vert := verts[0]
						for _, vert := range verts[1:] {
							if prev_vert.Mesh != vert.Mesh &&
								prev_vert.Vertex[0] == vert.Vertex[0] &&
								prev_vert.Vertex[1] == vert.Vertex[1] &&
								prev_vert.Vertex[2] == vert.Vertex[2] {
								// Found a matching vertex `vert`, which is shared between
								//  boundary1 and boundary2.
								// Record the mesh and index of both occurances of a vertex at
								//  this location.
								newVertexOccurances[*vert.Vertex] = append(
									newVertexOccurances[*vert.Vertex],
									prev_vert,
									vert,
								)
							}
							prev_vert = vert
						}
					}
				}

				// Send newly discovered vertex occurances to the reciever to be added
				// into the VertexOccurances map.
				new_vertex_occurances <- newVertexOccurances

			}(mesh_name1, mesh_name2, boundaries[mesh_name1], boundaries[mesh_name2])
		}
	}

	// Recieve new_boundaries until they're all done
	go func() {
		wg.Wait()
		close(new_vertex_occurances)
	}()
	for recieved_vertex_occurances := range new_vertex_occurances {
		for vertex, occurances := range recieved_vertex_occurances {
			vertexOccurances[vertex] = append(
				vertexOccurances[vertex],
				occurances...,
			)
		}
		wg.Done()
	}

	// Finally, unpack vertexOccurances to populate ss.BordersIndex and the
	//  Borders object of each MeshWrapper.
	new_mesh_border_vertices := make(chan map[string]map[string][]int, 16)
	for _, occurances := range vertexOccurances {
		wg.Add(1)
		go func(occurances []*vertexWrapper) {
			border_participants := make([]string, 0)
			mesh_border_vertices := make(map[string]map[string][]int)

			// uniqueify occurances of this vertex by mesh
			sort.Sort(vertsByMesh{occurances})
			unique_occurances := make([]*vertexWrapper, 0, len(occurances))
			var found bool
			for _, occurance := range occurances {
				found = false
				for _, uo := range unique_occurances {
					if occurance.MeshName == uo.MeshName {
						found = true
						break
					}
				}
				if !found {
					unique_occurances = append(unique_occurances, occurance)
				}
			}
			occurances = unique_occurances

			// Derive the canonical description for the border this vertex belongs to
			for _, occurance := range occurances {
				border_participants = append(border_participants, occurance.MeshName)
			}
			sort.Strings(border_participants)
			border_desc := strings.Join(border_participants, "_")

			// Register this vertex as being the next item in the determined border
			//  for each of the participating meshes.
			for _, occurance := range occurances {
				if _, exists := mesh_border_vertices[occurance.MeshName]; !exists {
					mesh_border_vertices[occurance.MeshName] = make(map[string][]int)
				}
				mesh_border_vertices[occurance.MeshName][border_desc] = append(
					mesh_border_vertices[occurance.MeshName][border_desc],
					occurance.Index,
				)
			}

			new_mesh_border_vertices <- mesh_border_vertices
		}(occurances)
	}

	// Recieve new_mesh_border_vertices until they're all done
	go func() {
		wg.Wait()
		close(new_mesh_border_vertices)
	}()
	for recieved_mesh_border_vertices := range new_mesh_border_vertices {
		for mesh_name, borders := range recieved_mesh_border_vertices {
			for border_desc, indices := range borders {
				// Lookup/Create am int-string border id for a border of this description
				border_id, border_exists := ss.BordersIndex[border_desc]
				if !border_exists {
					border_id = strconv.Itoa(len(ss.BordersIndex))
					ss.BordersIndex[border_desc] = border_id
				}
				ss.Meshes[mesh_name].Borders[border_id] = append(
					ss.Meshes[mesh_name].Borders[border_id],
					indices...,
				)
			}
		}
		wg.Done()
	}

	return
}

// Border Verification:
// --------------------
// The VerifyBorders method checks that all the indexed borders in the ShapeSet
// actually match between meshes, i.e. that for any given vertex in any border
// in a given mesh, there a counterpart at the same point in the border and the
// at the same cartesian location, in every other mesh sharing that border.

type collatedVertex map[string][3]float64

func (cv *collatedVertex) isValid() bool {
	first := true
	var prev [3]float64
	for _, vertex := range *cv {
		if first {
			prev = vertex
		} else if vertex != prev {
			return false
		}
	}
	return true
}

func (ss *ShapeSet) VerifyBorders() (err error) {
	// build up a list of border ids
	border_ids := make(map[string]int)
	for _, m := range ss.Meshes {
		for border_id, border_indices := range m.Borders {
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
							make(map[string][3]float64),
						)
					}
					vertex := [3]float64{}
					copy(vertex[:], ss.Meshes[mesh_name].Mesh.Verts.Get(vertex_index))
					all_borders[border_id][i][mesh_name] = vertex
				}
			}
		}
	}

	// Check all border verts are valid, issueing warnings if errors detected
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
	return
}
