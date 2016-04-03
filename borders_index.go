package shapeset

import (
	"github.com/nat-n/geom"
	"github.com/nat-n/gomesh/cuboid"
	gomesh "github.com/nat-n/gomesh/mesh"
	"sort"
	"sync"
)

type boundaryDetails struct {
	Verts       []gomesh.VertexI
	BoundingBox cuboid.Cuboid
}

type boundarySet struct {
	Boundaries []*boundaryDetails
	MeshId     MeshId
}

func (ss *ShapeSet) IndexBorders() (err error) {
	// Clear existing borders
	ss.ResetBorders()

	// Compose a map with the boundaries with bounding boxes for each mesh
	var wg sync.WaitGroup
	new_boundaries := make(chan *boundarySet, 16)
	for _, m := range ss.Meshes {
		wg.Add(1)
		go func(mesh1 *Mesh) {
			// Ensure vertices and faces know their places
			mesh1.ReindexVerticesAndFaces()

			m_boundaries, err := mesh1.IdentifyBoundaries()
			if err != nil {
				return
			}
			mesh1_boundaries := make([]*boundaryDetails, len(m_boundaries))
			for i, boundary := range m_boundaries {
				wrapped_boundary := make([]gomesh.VertexI, len(boundary))
				for j, bv := range boundary {
					wrapped_boundary[j] = bv
				}
				mesh1_boundaries[i] = &boundaryDetails{
					wrapped_boundary,
					*mesh1.SubsetBoundingBox(boundary),
				}
			}
			new_boundaries <- &boundarySet{mesh1_boundaries, MeshIdFromString(mesh1.Name)}
		}(m)
	}

	// Recieve new_boundaries until they're all done
	go func() {
		wg.Wait()
		close(new_boundaries)
	}()
	boundaries := make(map[MeshId][]*boundaryDetails)
	for new_boundary_set := range new_boundaries {
		boundaries[new_boundary_set.MeshId] = new_boundary_set.Boundaries
		wg.Done()
	}

	// The following block compares all borders to build up the vertexOccurances
	//  map of a location onto a number of vertices from different meshes
	vertexOccurances := make(map[geom.Vec3][]gomesh.VertexI)
	new_vertex_occurances := make(chan map[geom.Vec3][]gomesh.VertexI, 16)
	for mesh_id1, m1 := range ss.Meshes {
		for mesh_id2, m2 := range ss.Meshes {
			if mesh_id2.LessThan(mesh_id1) ||
				!m1.BoundingBox.Expanded(0.01).Intersects(
					m2.BoundingBox.Expanded(0.01)) {
				// Make sure we only deal with each pair of meshes once,
				//  and ignore pairs of meshes whose bounding boxes dont intersect
				continue
			}
			wg.Add(1)
			go func(mesh_id1, mesh_id2 MeshId, boundaries1, boundaries2 []*boundaryDetails) {
				newVertexOccurances := make(map[geom.Vec3][]gomesh.VertexI)
				for _, boundary1 := range boundaries1 {
					for _, boundary2 := range boundaries2 {
						if !boundary1.BoundingBox.Expanded(0.01).Intersects(
							boundary2.BoundingBox.Expanded(0.01)) {
							continue
						}
						// Identify any matching vertices between boundary1 and boundary2
						// by using a sort wrapper to order the verts so that potentially
						// colocated vertices occur consequtively.
						verts := make([]gomesh.VertexI, 0)

						// Loop over the vertices in both borders to fill in verts
						for _, vert := range boundary1.Verts {
							verts = append(verts, vert)
						}
						for _, vert := range boundary2.Verts {
							verts = append(verts, vert)
						}

						// Sort verts so that any border vertex from m2 that is potentially
						// collocated with a border vertex from m1 follows it directly in
						// the array.
						sort.Sort(gomesh.VerticesByPosition{verts})

						// iterate over the sorted verts and identify pairs of verts from
						// different meshes at the same location
						prev_vert := verts[0]
						for _, vert := range verts[1:] {
							// This assumes that these vertices both appear in only one mesh already!!
							// ... which will be true in the main use case.
							cm, _ := vert.GetMeshLocation()
							pm, _ := prev_vert.GetMeshLocation()
							if pm != cm &&
								prev_vert.GetX() == vert.GetX() &&
								prev_vert.GetY() == vert.GetY() &&
								prev_vert.GetZ() == vert.GetZ() {
								// Found a matching vertex `vert`, which is shared between
								//  boundary1 and boundary2.
								// Record the mesh and index of both occurances of a vertex at
								//  this location.
								vec := geom.Vec3{vert.GetX(), vert.GetY(), vert.GetZ()}
								newVertexOccurances[vec] = append(
									newVertexOccurances[vec],
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

			}(mesh_id1, mesh_id2, boundaries[mesh_id1], boundaries[mesh_id2])
		}
	}

	// Recieve new_boundaries until they're all done
	go func() {
		wg.Wait()
		close(new_vertex_occurances)
	}()
	for recieved_vertex_occurances := range new_vertex_occurances {
		// this channel recieves once for each overlapping pair of boundaries
		for vec, occurances := range recieved_vertex_occurances {
			vertexOccurances[vec] = append(vertexOccurances[vec], occurances...)
		}
		wg.Done()
	}

	// Finally, unpack vertexOccurances to populate ss.BordersIndex and the
	//  Borders object of each MeshWrapper.
	new_mesh_border_verts := make(chan map[MeshId]map[BorderDescription][]gomesh.VertexI, 16)
	for _, occurances := range vertexOccurances {
		wg.Add(1)
		go func(occurances []gomesh.VertexI) {
			border_participants := make([]MeshId, 0)
			mesh_border_verts := make(map[MeshId]map[BorderDescription][]gomesh.VertexI)

			// uniqueify occurances of this vertex by mesh
			sort.Sort(gomesh.VerticesByMesh{occurances})
			unique_occurances := make([]gomesh.VertexI, 0, len(occurances))
			var found bool
			for _, occurance := range occurances {
				found = false
				for _, uo := range unique_occurances {
					uo_m, _ := uo.GetMeshLocation()
					occurance_m, _ := occurance.GetMeshLocation()
					if occurance_m == uo_m {
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
				occurance_m, _ := occurance.GetMeshLocation()
				border_participants = append(border_participants,
					MeshIdFromString(occurance_m.GetName()))
			}
			sort.Sort(ByMeshIdPrecedence(border_participants))
			border_desc := BorderDescriptionFromMeshIds(border_participants)

			// Register this vertex as being the next item in the determined border
			//  for each of the participating meshes.
			for _, vert := range unique_occurances {
				occurance_m, _ := vert.GetMeshLocation()
				occurance_mid := MeshIdFromString(occurance_m.GetName())
				if _, exists := mesh_border_verts[occurance_mid]; !exists {
					mesh_border_verts[occurance_mid] = make(map[BorderDescription][]gomesh.VertexI)
				}
				mesh_border_verts[occurance_mid][border_desc] = append(
					mesh_border_verts[occurance_mid][border_desc],
					vert,
				)
			}

			new_mesh_border_verts <- mesh_border_verts
		}(occurances)
	}

	// Recieve new_mesh_border_verts until they're all done
	go func() {
		wg.Wait()
		close(new_mesh_border_verts)
	}()

	// A slice of all border descriptions as strings for the sake of sorting
	border_desc_strings := make([]string, 0)
	encountered_border_descs := make(map[BorderDescription]map[MeshId][]gomesh.VertexI)

	for recieved_mesh_border_verts := range new_mesh_border_verts {
		for mesh_id, border_verts := range recieved_mesh_border_verts {
			for border_desc, verts := range border_verts {
				// Lookup/Create am int-string border id for a border of this description
				_, border_seen := encountered_border_descs[border_desc]
				if !border_seen {
					border_desc_strings = append(border_desc_strings, border_desc.ToString())
					encountered_border_descs[border_desc] = make(map[MeshId][]gomesh.VertexI)
				}
				encountered_border_descs[border_desc][mesh_id] = append(
					encountered_border_descs[border_desc][mesh_id],
					verts...,
				)
			}
		}
		wg.Done()
	}

	// Create borders in string sorted order
	sort.Strings(border_desc_strings)
	for _, border_desc_str := range border_desc_strings {
		ss.BordersIndex.NewBorder(BorderDescriptionFromString(border_desc_str))
	}

	for border_desc, bmap := range encountered_border_descs {
		border := ss.BordersIndex.BorderFor(border_desc)

		// single out border vertices of first mesh to be the border vertices
		var first_mesh_verts []gomesh.VertexI
		for first_mesh_id, border_verts := range bmap {
			first_mesh_verts = border_verts
			delete(bmap, first_mesh_id)
			break
		}
		for _, v1I := range first_mesh_verts {
			v1 := v1I.(*Vertex)
			v1.Border = border
			border.Vertices = append(border.Vertices, v1)
		}

		// merge border vertices from other meshes into those of the first mesh
		for _, border_verts := range bmap {
			for vi, v1I := range first_mesh_verts {
				v2I := border_verts[vi]
				v1 := v1I.(*Vertex)
				v2 := v2I.(*Vertex)
				// move v2.Faces over to v1.Faces
				err := gomesh.MergeSharedVertices(v1I, v2I)
				if err != nil {
					panic(err)
				}
				// move v2.Edges over to v1.Edges
				for _, e := range v2.Edges {
					e.ReplaceVertex(v2, v1)
					v1.AddEdge(e)
				}
				v2.Edges = v2.Edges[:0]
				v2_mesh, v2_i := v2.GetMeshLocation()
				v2_mesh.GetVertices().Update(v2_i, v1I)
				v1.SetLocationInMesh(&v2_mesh, v2_i)
			}
		}
	}

	// Now that border vertices are registered, infer border edges
	ss.BordersIndex.indexBorderEdges()

	return
}
