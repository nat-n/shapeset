package shapeset

import (
	"fmt"
	"github.com/nat-n/geom"
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
//       can then iterate over the resulting sorted collection to find pairs of
//       vertices where the first is from m1, and the second from m2 and their
//  		 and x,y,z values are identical.

type vertexWrapper struct {
	Vertex   *geom.Vec3
	Mesh     int // 0 for mesh1 or 1 for mesh2
	MeshName string
	Index    int // index of this vertex in the m1.Verts or m2.Verts
}

type boundaryDetails struct {
	Verts       *[]*vertexWrapper
	BoundingBox cuboid.Cuboid
}

type boundarySet struct {
	Boundaries []*boundaryDetails
	MeshName   string
}

type sortableVertices []*vertexWrapper

func (vs sortableVertices) Len() int      { return len(vs) }
func (vs sortableVertices) Swap(i, j int) { vs[i], vs[j] = vs[j], vs[i] }

type vertsByPosition struct{ sortableVertices }

func (vs vertsByPosition) Less(i, j int) bool {
	a := vs.sortableVertices[i]
	b := vs.sortableVertices[j]
	if a.Vertex.X > b.Vertex.X {
		return false
	} else if a.Vertex.X == b.Vertex.X {
		if a.Vertex.Y > b.Vertex.Y {
			return false
		} else if a.Vertex.Y == b.Vertex.Y {
			if a.Vertex.Z > b.Vertex.Z {
				return false
			} else if a.Vertex.Z == b.Vertex.Z {
				if a.Mesh > b.Mesh {
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
	for _, mw := range ss.Meshes {
		// for some reason simply reassigning mw.Borders doesn't work!
		for border_id, _ := range mw.Borders {
			delete(mw.Borders, border_id)
		}
	}

	fmt.Println("Identifying border vertices")

	boundaries := make(map[string][]*boundaryDetails)

	// Compose a map with the boundaries with bounding boxes for each mesh
	var wg sync.WaitGroup
	new_boundaries := make(chan *boundarySet, 16)
	for _, m := range ss.Meshes {
		wg.Add(1)
		go func(mesh1 *mesh.Mesh) {
			m_boundaries := mesh1.IdentifyBoundaries()
			mesh1_boundaries := make([]*boundaryDetails, len(m_boundaries))
			for i, boundary := range m_boundaries {
				wrapped_boundary := make([]*vertexWrapper, len(boundary))
				for j, bv := range boundary {
					wrapped_boundary[j] = &vertexWrapper{
						Vertex:   mesh1.Verts.Get(bv)[0],
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
		fmt.Print(".")
		boundaries[new_boundary_set.MeshName] = new_boundary_set.Boundaries
		wg.Done()
	}

	fmt.Println("Matching up border vertices" + strconv.Itoa(len(boundaries)))

	// The following block compares all borders to build up the vertexOccurances
	//  map of a location onto a number of vertices from different meshes
	vertexOccurances := make(map[geom.Vec3][]*vertexWrapper)
	new_vertex_occurances := make(chan map[geom.Vec3][]*vertexWrapper, 16)
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
				newVertexOccurances := make(map[geom.Vec3][]*vertexWrapper)
				for _, boundary1 := range boundaries1 {
					for _, boundary2 := range boundaries2 {
						if !boundary1.BoundingBox.Expanded(0.01).Intersects(
							boundary2.BoundingBox.Expanded(0.01)) {
							continue
						}
						// Now we need to identify any matching vertices between boundary1
						// and boundary2.
						// This is done using a sort wrapper to order the verts so that
						// potentially colocated vertices occur consequtively
						verts := make([]*vertexWrapper, 0)

						// Loop over the vertices in both borders to fill in verts
						for _, wrapped_vert := range *boundary1.Verts {
							verts = append(verts, &vertexWrapper{
								Vertex:   wrapped_vert.Vertex,
								Mesh:     0,
								MeshName: mesh_name1,
								Index:    wrapped_vert.Index,
							})
						}
						for _, wrapped_vert := range *boundary2.Verts {
							verts = append(verts, &vertexWrapper{
								Vertex:   wrapped_vert.Vertex,
								Mesh:     1,
								MeshName: mesh_name2,
								Index:    wrapped_vert.Index,
							})
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
								prev_vert.Vertex.X == vert.Vertex.X &&
								prev_vert.Vertex.Y == vert.Vertex.Y &&
								prev_vert.Vertex.Z == vert.Vertex.Z {
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
		// this channel recieves once for each overlapping pair of boundaries
		for vertex, occurances := range recieved_vertex_occurances {
			vertexOccurances[vertex] = append(
				vertexOccurances[vertex],
				occurances...,
			)
		}
		wg.Done()
	}

	fmt.Println("Building BordersIndex with " + strconv.Itoa(len(vertexOccurances)))

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

			// Derive the canonical description for the border this vertex belongs to
			for _, occurance := range unique_occurances {
				border_participants = append(border_participants, occurance.MeshName)
			}
			sort.Strings(border_participants)
			border_desc := strings.Join(border_participants, "_")

			// Register this vertex as being the next item in the determined border
			//  for each of the participating meshes.
			for _, occurance := range unique_occurances {
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

	// Build ss.BordersIndex as a map of border descriptions onto temporary
	// border_ids. Use temporary border ids initially determined by the order
	// that the new_mesh_border_vertices channel yeilds data containing each
	// border description.

	// A slice of all border descriptions
	all_border_descs := make([]string, 0)

	for recieved_mesh_border_vertices := range new_mesh_border_vertices {
		for mesh_name, borders := range recieved_mesh_border_vertices {
			for border_desc, indices := range borders {
				// Lookup/Create am int-string border id for a border of this description
				_, border_seen := ss.BordersIndex[border_desc]
				if !border_seen {
					// 	temp_border_id = strconv.Itoa(len(ss.BordersIndex))
					// ss.BordersIndex[border_desc] = temp_border_id
					ss.BordersIndex[border_desc] = border_desc
					all_border_descs = append(all_border_descs, border_desc)
				}
				ss.Meshes[mesh_name].Borders[border_desc] = append(
					ss.Meshes[mesh_name].Borders[border_desc],
					indices...,
				)
			}
		}
		wg.Done()
	}

	fmt.Println("Assigning border ids for " + strconv.Itoa(len(all_border_descs)))

	// Assign deterministic ids for each border, as the index of the border
	// description in the sorted array of all border descriptions.
	// Update all border id references in ss.BordersIndex and
	// ss.Meshes[mesh_name].Borders.
	// We do this in order to make the result output more idempotent.
	sort.Strings(all_border_descs)

	for i, border_desc := range all_border_descs {
		border_id := strconv.Itoa(i)
		ss.BordersIndex[border_desc] = border_id
	}

	for _, mw := range ss.Meshes {
		for _, border_desc := range all_border_descs {
			if _, exists := mw.Borders[border_desc]; exists {
				mw.Borders[ss.BordersIndex[border_desc]] = mw.Borders[border_desc]
				delete(mw.Borders, border_desc)
			}
		}
	}

	return
}
