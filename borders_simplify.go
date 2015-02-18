package shapeset

import "strings"
import "fmt"
import "container/heap"
import "github.com/nat-n/gomesh/mesh"
import "github.com/nat-n/geom"

/*
 * quadric edge collapse simplification of border edges accross all borders
 */

type vertex struct {
	geom.Vec3
	Q             *geom.SymMat4
	Edges         []*edge
	Instances     map[*mesh.Mesh]int
	CollapsedInto *vertex
}

type edge struct {
	V1             *vertex
	V2             *vertex
	CollapseTarget geom.Vec3
	Q              *geom.SymMat4
	Error          float64
	Collapsed      bool
	Protected      bool
}

// Calculate the Kp fundemental error matrix of a face, i.e. quadric of plane
func calculateKp(v1, v2, v3 *geom.Vec3) *geom.SymMat4 {
	// calcualte face normal
	n := geom.TriNormal(v1, v2, v3)
	a, b, c := n.X, n.Y, n.Z

	// use center point of triangle is better?
	cx := (v1.X + v2.X + v3.X) / 3
	cy := (v1.Y + v2.Y + v3.Y) / 3
	cz := (v1.Z + v2.Z + v3.Z) / 3
	d := -(a*cx + b*cy + c*cz)

	return &geom.SymMat4{
		a * a, a * b, a * c, a * d,
		b * b, b * c, b * d,
		c * c, c * d,
		d * d,
	}
}

//
//   .  -  =  ==  ===  Psudo code Overview of Procedure  ===  ==  =  -  .
//
// # Preparing edges
//    - collect border vertices accross meshes
// 		- using just the first mesh
// 			- create a map of border verts := map[int]vertex
// 			- for each vertex in the border:
// 				- find neighbors with lower value indices and look them up in the map
// 				- for each neighbor found in the map: create an edge
//
// 		# Determine edge collapsibility & calculate initial vertex error quadrics
// =>	- for each border:
// 			- for each vertex in border:
// 				- for each mesh in border:
// 					- for each face of the vertex in the mesh:
// 						- if it includes a border edge:
// 							- if it includes a third vertex on this border:
// 								- mark this edge as non-collapsible
// 							- else:
// 								- calculate the Kp and add it to the error for this vertex
// 									- if the other vertex on the edge doesn't have any error yet:
// 										- add the Kp to that vertex too					<== NOT SURE OF LOGIC HERE
// 						- else:
// 							- calculate the Kp and add it to the error for this vertex
//
// 		# Prepare edges for collapsing
// 		- for each edge:
//			- if it has not been marked as non-collapsible:
//				- determine the optimal collapse location and error from the verts
// 			- else:
// 				- remove it from the collection
//
// 		- initalise/sort heap of edges
//
// # Simplifying meshes
// 		- until the number of edges has been reduced by a certain threshold
// 			- pop off the lowest error edge from the heap
// 			- collapse it
// 			- reposition affected edges in the heap
// 		- apply changes back to meshes and borders
//
// # Collapsing an edge
// 		- mark V2 as collapsed into V1
// 		- update V1 to inherit V2's other edge (if any)
// 		- update V2's other edge (if any) to reference V1
// 		- reposition V1 to the collapse target position of the edge
// 		- recalculate collapse targets and errors for both edges now referencing
// 			V1, and reposition them in the heap.
//
// # Applying changes back to meshes and border
// 		- initialise map like map[int]int for vertices to point from old index to
// 		  new index
// 		- for each border vertex:
// 			- if the border vertex has been collapsed:
// 				- add an entry into the vertex index map
// 			- else:
//   			- for each mesh of the vertex: update the position of the vertex
//   	- create a set of removed vertex indices
// 		- filter the border index to remove references to collapsed vertices
// 		- for each vertex in the mesh:
// 			- if it is in the set of removed vertices:
// 				- increment the count of removed vertices
// 			- else:
// 				- add an entry to the map of the vertex's current index to the current
// 					index minus the current count of removed vertices
// 		- Update every entry in the mesh's faces buffer using the map
// 		- Filter out all collapsed vertices from the mesh's vertex buffer
// 			(and vertex normals)

func (ss *ShapeSet) SimplifyBorders() (err error) {

	// These could be made into arguments instead
	const error_threshold = 1.0
	const aggressiveness = 0.95
	const forgiveness = 5

	// ensure  we have up to date indexes on all face buffers
	for _, mw := range ss.Meshes { // parallelise this... encapsulate as method on ss??
		func(mw MeshWrapper) {
			mw.Mesh.Faces.UpdateIndex()
		}(mw)
	}

	// An array with an edgeHeap for each border
	border_heaps := make([]*edgeHeap, 0)

	// A map with an array with a vertex object for each border vertex for each
	// border_id
	border_vert_index := make(map[string][]*vertex)

	// Collect border vertices across meshes and find edges
	for border_desc, border_id := range ss.BordersIndex {

		func(border_desc, border_id string) {
			// create collection for edges heap
			edges := &edgeHeap{}

			// ids of meshes in this border
			border_mesh_ids := strings.Split(border_desc, "_")
			border_length := len(ss.Meshes[border_mesh_ids[0]].Borders[border_id])
			if border_length < 2 {
				// nothing to be done with single point (no edge) borders
				return
			}
			first_mesh_wr := ss.Meshes[border_mesh_ids[0]]
			first_mesh_border := first_mesh_wr.Borders[border_id]

			// map from border vertex location in the first mesh, onto vertex object
			border_vert_index[border_id] = make([]*vertex, 0)

			// Create vertex objects for vertices of this border
			for i := 0; i < border_length; i++ {
				// initialise the vertex with coordinates from the first mesh
				new_vert := &vertex{
					Vec3:      *first_mesh_wr.Mesh.Verts.Get(first_mesh_border[i])[0],
					Instances: make(map[*mesh.Mesh]int),
					Q:         &geom.SymMat4{},
					Edges:     make([]*edge, 0),
				}

				// For each mesh of this boreder,
				//  add a vertex reference ( *Mesh => vi ) to new_vert
				for _, mesh_id := range border_mesh_ids {
					vi := ss.Meshes[mesh_id].Borders[border_id][i]
					new_vert.Instances[ss.Meshes[mesh_id].Mesh] = vi
				}

				border_vert_index[border_id] = append(
					border_vert_index[border_id],
					new_vert,
				)
			}

			// Calculate initial vertex error quadrics from faces Kp
			for i := 0; i < border_length; i++ {
				for _, mesh_id := range border_mesh_ids {
					vi := ss.Meshes[mesh_id].Borders[border_id][i]
					vi_face_indices := ss.Meshes[mesh_id].Mesh.Faces.Index[vi]
					ss.Meshes[mesh_id].Mesh.Faces.EachOf(func(a, b, c int) {
						// Calculate Kp for this face and add it to this vertex
						face_kp := calculateKp(
							ss.Meshes[mesh_id].Mesh.Verts.Get(a)[0],
							ss.Meshes[mesh_id].Mesh.Verts.Get(b)[0],
							ss.Meshes[mesh_id].Mesh.Verts.Get(c)[0],
						)

						vertex_obj := border_vert_index[border_id][i]
						vertex_obj.Q.Add(face_kp)
					}, vi_face_indices...)
				}
			}

			// Find edges
			for i1 := 0; i1 < border_length; i1++ {
				// Using the first mesh (first_mesh_wr) as an exemplar, find neighboring
				// vertices of vertex i in the border (with lower ordinal value in the
				// first mesh, to avoid matching the same two vertices twice) and create
				// an edge between them.

				// lookup ordinal index of ith border vertex in the first mesh
				vi := first_mesh_wr.Borders[border_id][i1]
				// lookup indices of faces including this vertex
				face_indices := first_mesh_wr.Mesh.Faces.Index[vi]
				// find complete set of vertex (index) occurances in these faces
				faces_contents := first_mesh_wr.Mesh.Faces.Get(face_indices...)
				// count the occurances of other vertices in this set (neighbors of vi)
				neighbor_counts := make(map[int]int)
				for _, ni := range faces_contents {
					neighbor_counts[ni] += 1
				}

				// If a neighbor vertex occurs in exactly one other face then the edge
				// from vi to the other vertex is a boundary edge, and if the other
				// vertex occurs on this border then the edge is an edge of this border
				// so should be saved (unless the other vertex has a higher ordinal
				// than this vertex, in which case it's ignored now because it will be
				// captured when the the other vertex is examined).

				for ni, count := range neighbor_counts {
					if count != 1 || ni >= vi {
						// Skip unless vi-ni is a boundary edge (i.e. they occur
						// together in exactly one face in the first mesh)
						continue
					}

					// find the location of ni in the border to get its vertex object
					found_index := -1
					for j := 0; j < border_length; j++ {
						if first_mesh_border[j] == ni {
							found_index = j
							break
						}
					}
					if found_index == -1 {
						// these vertices share an edge but not on this border
						continue
					}
					i2 := found_index

					if i1 == i2 {
						panic("Model assumption violated: invalid edge encountered")
					}

					// Create an edge from i1 to i2
					new_edge := &edge{
						V1:        border_vert_index[border_id][i1],
						V2:        border_vert_index[border_id][i2],
						Collapsed: false,
					}

					new_edge.V1.Edges = append(new_edge.V1.Edges, new_edge)
					new_edge.V2.Edges = append(new_edge.V2.Edges, new_edge)

					new_edge.calculateError()
					edges.Push(new_edge)
				}
			}

			// only borders with > 5 edges are worth trying to decimate
			if edges.Len() > 5 {
				// Sort edges by error and save for later if enough edges were found
				heap.Init(edges)
				border_heaps = append(border_heaps, edges)
			}

		}(border_desc, border_id)
	}

	// for each border (in parallel)
	// iteratively collapse edges until threshold reached
	for _, edges := range border_heaps {
		func(edges *edgeHeap) {
			ideal_edge_count := int(float64(edges.Len())*aggressiveness) - forgiveness
			for i := 0; i < ideal_edge_count; i++ {
				lowest_cost_edge := heap.Pop(edges).(*edge)
				if lowest_cost_edge.Error > error_threshold {
					// Quadric error of remaining edges is too high so don't collapse any
					// more
					return
				}
				affected_edges := lowest_cost_edge.collapse()
				edges.UpdateEdges(affected_edges)
			}
		}(edges)
	}

	// ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ---- ----

	// Finally: unpack vertices and apply diff to meshes and borders
	// consisting of removing verts and updating faces (probably like compose)
	// also remove faces including a collapsed edge

	for _, mw := range ss.Meshes {
		// iterating over meshes less efficient but easier to parallelise
		func(mw MeshWrapper) {
			// create map of collapsed border vertex indices onto the indices of
			// their collapse targets
			collapse_map := make(map[int]int)
			for border_id, border := range mw.Borders {
				if len(border) < 2 {
					// We're not interested in really short borders
					continue
				}

				for i := 0; i < len(border); i++ {
					v := border_vert_index[border_id][i]
					if v.CollapsedInto != nil {
						collapse_target := v.CollapsedInto

						// Find the final collapse destination, in case collapse_target was
						// further collapsed
						for true {
							// logically possible to infinite loop here... though implausible
							if collapse_target.CollapsedInto == nil {
								collapse_map[v.Instances[mw.Mesh]] = collapse_target.Instances[mw.Mesh]
								break
							}
							collapse_target = collapse_target.CollapsedInto
						}
					}
				}
			}

			// Build index_map to map from old vertex indices onto new
			index_map := make([]int, mw.Mesh.Verts.Len())
			new_i := 0
			for i := 0; i < mw.Mesh.Verts.Len(); i++ {
				if _, collapsed := collapse_map[i]; !collapsed {
					index_map[i] = new_i
					new_i++
				}
			}
			for collapsed, collapse_target := range collapse_map {
				index_map[collapsed] = index_map[collapse_target]
			}

			// Build collapsed_verts as list of verts to be removed from this mesh
			collapsed_verts := make([]int, 0)
			for collapsed_i, _ := range collapse_map {
				collapsed_verts = append(collapsed_verts, collapsed_i)
			}
			// Remove collapsed_verts
			mw.Mesh.Verts.Remove(collapsed_verts...)

			// Remap remaining faces using index_map, and identify collapsed faces
			collapsed_faces := make([]int, 0)
			face_i := 0
			mw.Mesh.Faces.Collect(func(a, b, c int) (int, int, int) {

				// make sure index_map values exist,
				//  though this it too costly work to leave active
				// if index_map[a] >= mw.Mesh.Verts.Len() {
				// 	panic("Invalid remapping of face vertex (" + strconv.Itoa(a) +
				// 		" mapped to " + strconv.Itoa(index_map[a]) + " of " +
				// 		strconv.Itoa(mw.Mesh.Verts.Len()) + ")")
				// }
				// if index_map[b] >= mw.Mesh.Verts.Len() {
				// 	panic("Invalid remapping of face vertex (" + strconv.Itoa(b) +
				// 		" mapped to " + strconv.Itoa(index_map[b]) + " of " +
				// 		strconv.Itoa(mw.Mesh.Verts.Len()) + ")")
				// }
				// if index_map[c] >= mw.Mesh.Verts.Len() {
				// 	panic("Invalid remapping of face vertex (" + strconv.Itoa(c) +
				// 		" mapped to " + strconv.Itoa(index_map[c]) + " of " +
				// 		strconv.Itoa(mw.Mesh.Verts.Len()) + ")")
				// }

				mappedA := index_map[a]
				mappedB := index_map[b]
				mappedC := index_map[c]
				if mappedA == mappedB || mappedA == mappedC || mappedB == mappedC {
					// this face has been collapsed so should be removed
					collapsed_faces = append(collapsed_faces, face_i)
				}
				face_i++
				return mappedA, mappedB, mappedC
			})

			// Remove collapsed_faces
			mw.Mesh.Faces.Remove(collapsed_faces...)

		}(mw)
	}

	// Clear borders indexes, since they will no longer be valid
	ss.BordersIndex = make(map[string]string)
	for _, mw := range ss.Meshes {
		// for some reason simply reassigning mw.Borders doesn't work!
		for border_id, _ := range mw.Borders {
			delete(mw.Borders, border_id)
		}
	}

	return
}

func (e *edge) calculateError() {
	// Calculate error quadric for this edge as sum of vertex error quadrics
	Q := geom.SymMat4{}
	e.Q = &Q
	e.Q.Add(e.V1.Q)
	e.Q.Add(e.V2.Q)

	// Optimised determinant formula when the bottom row of Q is subsituted for [0, 0, 0, 1]
	det := -Q[2]*Q[4]*Q[2] + Q[1]*Q[5]*Q[2] + Q[2]*Q[1]*Q[5] - Q[0]*Q[5]*Q[5] - Q[1]*Q[1]*Q[7] + Q[0]*Q[4]*Q[7]
	if det != 0 {
		// Optimised implementation of Q^-1 * {{0},{0},{0},{1}}
		e.CollapseTarget.X = (Q[3]*Q[5]*Q[5] - Q[2]*Q[6]*Q[5] - Q[3]*Q[4]*Q[7] + Q[1]*Q[6]*Q[7] + Q[2]*Q[4]*Q[8] - Q[1]*Q[5]*Q[8]) / det
		e.CollapseTarget.Y = (Q[2]*Q[6]*Q[2] - Q[3]*Q[5]*Q[2] + Q[3]*Q[1]*Q[7] - Q[0]*Q[6]*Q[7] - Q[2]*Q[1]*Q[8] + Q[0]*Q[5]*Q[8]) / det
		e.CollapseTarget.Z = (Q[3]*Q[4]*Q[2] - Q[1]*Q[6]*Q[2] - Q[3]*Q[1]*Q[5] + Q[0]*Q[6]*Q[5] + Q[1]*Q[1]*Q[8] - Q[0]*Q[4]*Q[8]) / det
	} else {

		// Determine which is best, V1, V2 or their midpoint
		midpoint := geom.Vec3{
			(e.V1.X + e.V2.X) / 2,
			(e.V1.Y + e.V2.Y) / 2,
			(e.V1.Z + e.V2.Z) / 2,
		}
		v1_error := Q.VertexError(e.V1.Vec3)
		v2_error := Q.VertexError(e.V2.Vec3)
		midpoint_error := Q.VertexError(midpoint)

		if v1_error < v2_error {
			if v1_error < midpoint_error {
				e.CollapseTarget = e.V1.Vec3
			} else {
				e.CollapseTarget = midpoint
			}
		} else {
			if v2_error < midpoint_error {
				e.CollapseTarget = e.V2.Vec3
			} else {
				e.CollapseTarget = midpoint
			}
		}
	}

	e.Error = Q.VertexError(e.CollapseTarget)
}

func (e *edge) collapse() (recalculated []*edge) {
	// 		- mark e as collapsed
	// 		- mark V2 as collapsed into V1
	// 		- reposition V1 to the collapse target position of the edge
	// 		- update V1 to inherit V2's other edge(s?) (if any)
	// 		- update V2's other edge(s?) (if any) to reference V1
	// 		- recalculate collapse targets and errors for both edges now referencing
	// 			V1, and reposition them in the heap.

	// Keep track of other edges affected by the collapse of e
	if e.Protected {
		return
	}

	// Check if this edge has two adjacent edges that connect to form a triangle
	// If so then we want to mark all three of them as Protected to avoid problems
	adjacentVertsViaV1 := make(map[*vertex][]*edge)
	adjacentVertsViaV2 := make(map[*vertex][]*edge)
	for _, v1_edge := range e.V1.Edges {
		if v1_edge != e {
			if v1_edge.V1 != e.V1 {
				adjacentVertsViaV1[v1_edge.V1] = append(
					adjacentVertsViaV1[v1_edge.V1],
					v1_edge,
				)
			} else if v1_edge.V2 != e.V1 {
				adjacentVertsViaV1[v1_edge.V2] = append(
					adjacentVertsViaV1[v1_edge.V2],
					v1_edge,
				)
			} else {
				panic("Model assumption violated: v1_edge features e.V1 twice")
			}
		}
	}
	for _, v2_edge := range e.V2.Edges {
		if v2_edge != e {
			if v2_edge.V1 != e.V2 {
				adjacentVertsViaV2[v2_edge.V1] = append(
					adjacentVertsViaV2[v2_edge.V1],
					v2_edge,
				)
			} else if v2_edge.V2 != e.V2 {
				adjacentVertsViaV2[v2_edge.V2] = append(
					adjacentVertsViaV2[v2_edge.V2],
					v2_edge,
				)
			} else {
				panic("Model assumption violated: v2_edge features e.V2 twice")
			}
		}
	}
	for adjvert1, adjvert1edges := range adjacentVertsViaV1 {
		if len(adjvert1edges) != 1 {
			panic("Model assumption violated: two edges from e.V1 to adjvert1")
		}
		for adjvert2, adjvert2edges := range adjacentVertsViaV2 {
			if len(adjvert2edges) != 1 {
				panic("Model assumption violated: two edges from e.V2 to adjvert2")
			}
			if adjvert1 == adjvert2 {
				// Small loop found, protect involved edges and cancel collapse
				e.Protected = true
				adjvert1edges[0].Protected = true
				adjvert2edges[0].Protected = true
				return
			}
		}
	}

	// TODO: IT SEEMS WE NEED TO EXTEND THIS LOGIC TO WHETHER e FORMS PART OF A SQUARE WITH THREE OTHER EDGES!!!

	/*
	 * say this edge is FC, so C will be collapsed into F
	 * - does C have another neighbor B that is a neghbor of F (not H tho...?)
	 or is it that C has more than one shared neighbor with F...
	 So, we'd check for neighbors of c (e.V2) how many occur in a face with F?
	 ... there will be at least one, but if there are two then that might be a problem.
	*/

	// - for each mesh
	// 		- find neighbors of e.V2
	// 		- count how many of neighbors share a face with e.V1
	//		 : only need to check indexes
	for m, i := range e.V2.Instances {
		v1_faces := m.Faces.Index[e.V1.Instances[m]]
		v2_neighbours := m.Faces.NeighboursOf(i)
		// fmt.Println(v2_neighbours)
		link_count := 0
		for _, nv := range v2_neighbours {
			if nv == e.V1.Instances[m] {
				// fmt.Println("QUACK")
				continue
			}
			n_faces := m.Faces.Index[nv]
			// fmt.Println(n_faces, v1_faces)
			link_count += func(n_faces, v1_faces []int) int {
				for _, nf := range n_faces {
					for _, v1f := range v1_faces {
						if nf == v1f {
							return 1
						}
					}
				}
				return 0
			}(n_faces, v1_faces)
		}
		if link_count > 1 {
			fmt.Println("::", link_count)
			e.Protected = true
			return
		}
	}
	// a neighbor of e.V2 shared two faces with e.V1... (in the original mesh)

	for m, i := range e.V2.Instances {
		v1_faces := m.Faces.Index[e.V1.Instances[m]]
		v2_faces := m.Faces.Index[e.V2.Instances[m]]
	}

	// I dont think we can be sure of anything without examing the up to date mesh,
	// which means we need to track all edges and vertices!!

	// Mark e as collapsed
	if e.Collapsed {
		panic("Model assumption violated: Edge already collapsed.")
	}
	e.Collapsed = true

	// mark e.V2 as collasped into e.V1
	if e.V1.CollapsedInto != nil {
		panic("Model assumption violated: Shouldn't be trying to collapse into collapsed vertex.")
	}
	if e.V2.CollapsedInto != nil {
		// As if some refernce from an edge to V2 was not updated in a previous
		// collapse...
		panic("Model assumption violated: Vertex already collapsed.")
	}
	e.V2.CollapsedInto = e.V1

	// Update position of e.V1
	e.V1.Vec3 = e.CollapseTarget

	// Find and remove e from V1.Edges
	v1e_i := -1
	for i, v1_edge := range e.V1.Edges {
		if v1_edge == e {
			if v1e_i >= 0 {
				panic("Model assumption violated: e.V1 references e more than once.")
			}
			v1e_i = i
		}
	}
	if v1e_i < 0 {
		panic("Model assumption violated: e.V1.Edges doesn't reference e.")
	}
	e.V1.Edges = append(e.V1.Edges[:v1e_i], e.V1.Edges[v1e_i+1:]...)
	// Find and remove e from V2.Edges
	v2e_i := -1
	for i, v2_edge := range e.V2.Edges {
		if v2_edge == e {
			if v2e_i >= 0 {
				panic("Model assumption violated: e.V2 references e more than once.")
			}
			v2e_i = i
		}
	}
	if v2e_i < 0 {
		panic("Model assumption violated: e.V2.Edges doesn't reference e.")
	}
	e.V2.Edges = append(e.V2.Edges[:v2e_i], e.V2.Edges[v2e_i+1:]...)

	// Transfer edges of V2 to V1
	// i.e. update all items in e.V2.Edges and add them to e.V1.Edges (except e),
	// also mark them as needed prioritisation
	for _, v2_edge := range e.V2.Edges {

		// Update v2_edge to reference e.V1 in place of e.V2
		if v2_edge.V1 == e.V2 {
			v2_edge.V1 = e.V1
		} else if v2_edge.V2 == e.V2 {
			v2_edge.V2 = e.V1
		} else {
			panic("Model assumption violated: Vertex to edge ref not reciprocated")
		}

		// Add reference from e.V1 to v2_edge
		e.V1.Edges = append(e.V1.Edges, v2_edge)

		// Recalculate error for modified edge
		v2_edge.calculateError()
		recalculated = append(recalculated, v2_edge)
	}

	return
}

//
// Interface and Convenience functions to for our heap of edges (should be in own file)
//

type edgeHeap []*edge

func (h edgeHeap) Len() int           { return len(h) }
func (h edgeHeap) Less(i, j int) bool { return h[i].Error < h[j].Error }
func (h edgeHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *edgeHeap) Push(x interface{}) {
	e := x.(*edge)
	*h = append(*h, e)
}

func (h *edgeHeap) Replace(es []*edge) {
	old := *h
	*h = old[:len(es)]
	copy(*h, es)
}

func (h *edgeHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func (h *edgeHeap) Fix(indices ...int) {
	for _, index := range indices {
		heap.Fix(h, index)
	}
}

// Find and call fix on each of the affected edges,
// this is inefficient, but not too bad, and I'm not sure how to avoid it
func (h *edgeHeap) UpdateEdges(affected_edges []*edge) {
	possible_matches := make([]*edge, len(affected_edges))
	copy(possible_matches, affected_edges)
	eh := *h
	for i, e := range eh {
		for j, possible_match := range possible_matches {
			if e == possible_match {
				heap.Fix(h, i)
				possible_matches = append(
					possible_matches[:j],
					possible_matches[j+1:]...,
				)
				break
			}
		}
	}
}
