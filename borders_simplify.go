package shapeset

import (
	"container/heap"
	"fmt"
	gomesh "github.com/nat-n/gomesh/mesh"
)

/*
 * quadric edge collapse simplification of border edges accross all borders
 */

func (ss *ShapeSet) SimplifyBorders(error_threshold, aggressiveness float64, forgiveness int) (err error) {
	// calculate face, vertex Kp error Quadrics for borders
	border_face_set := make(map[*Face]bool)
	ss.BordersIndex.Each(func(border *Border) {
		for _, v := range border.Vertices {
			v.EachFace(func(fx gomesh.FaceI) {
				f := fx.(*Face)
				if !border_face_set[f] {
					f.Kp = f.calculateKp()
					border_face_set[f] = true
				}
			})
		}
	})
	ss.BordersIndex.Each(func(border *Border) {
		for _, v := range border.Vertices {
			v.calculateError()
		}
	})

	// Create heaps of border edges
	edgeHeaps := make(map[BorderId]*edgeHeap)
	ss.BordersIndex.Each(func(border *Border) {
		edgeHeaps[border.Id] = &edgeHeap{}
		for _, e := range border.Edges {
			e.calculateError()
			edgeHeaps[border.Id].Push(e)
		}
		// Sort edges by error
		heap.Init(edgeHeaps[border.Id])
	})

	ss.BordersIndex.Each(func(border *Border) {
		for _, v := range border.Vertices {
			if v.Border == nil {
				fmt.Println(v.Border)
			}
		}
	})

	for x, edges := range edgeHeaps { // TODO: parallelise this
		if debug_level() >= 1 {
			fmt.Println("Creating heap of edges for border:", ss.BordersIndex.BorderFor(x).Description())
		}
		applySimplification(edges, error_threshold, aggressiveness, forgiveness)
	}

	// filter out collaposed stuff
	for _, m := range ss.Meshes {
		m.Faces.Filter(func(f gomesh.FaceI) bool { return !f.(*Face).Collapsed })
		m.Vertices.Filter(func(v gomesh.VertexI) bool { return v.(*Vertex).CollapsedInto == nil })
		m.ReindexVerticesAndFaces()
	}

	return
}

/* Collapses the provided edges in order of least error, until a stopping
 *  condition is reached.
 * aggressiveness: the portion of edges to attempt to collapse. 0 < a < 1
 * error_threshold: the maximum error that will be tolerated for an edge
 *  collapse to be attempted.
 * forgiveness: the number of edges less to be left short of the aggressiveness
 *  ratio.
 */
func applySimplification(edges *edgeHeap, error_threshold, aggressiveness float64, forgiveness int) {
	// determine desired number of edges to collapse
	edge_collapse_goal := int(float64(edges.Len())*aggressiveness) - forgiveness

	if debug_level() >= 1 {
		fmt.Println("Simplification goal to reduce edge count from",
			edges.Len(), "to", edge_collapse_goal)
	}

	for i := 0; i < edge_collapse_goal; i++ {
		lowest_cost_edge := heap.Pop(edges).(*Edge)

		if lowest_cost_edge.Error > error_threshold {
			// Quadric error of remaining edges is too high so stop collapsing
			if debug_level() >= 1 {
				fmt.Println("Quadric error of remaining edges is too high so stop collapsing")
			}
			return
		}

		affected_edges := lowest_cost_edge.Collapse()
		edges.UpdateEdges(affected_edges)
	}
}
