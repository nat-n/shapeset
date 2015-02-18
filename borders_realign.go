package shapeset

import "strings"

// Realign Borders:
// ----------------
// The RealignBorders method ensures that vertices that are shared between
// meshes according to the BordersIndex are indeed aligned. This is done by
// iterating through each vertex of each border, and repositioning all instance
// of each border vertex accross all the meshes that share it, to their average
// position.
func (ss *ShapeSet) RealignBorders() {
	for border_desc, border_id := range ss.BordersIndex {
		mesh_ids := strings.Split(border_desc, "_")

		// Determine average positions across meshes of vertices in this border
		border_length := len(ss.Meshes[mesh_ids[0]].Borders[border_id])
		average_border := make([]float64, border_length*3)
		for _, mesh_id := range mesh_ids {
			// mesh_id := mesh_ids[0]
			for i, vi := range ss.Meshes[mesh_id].Borders[border_id] {
				coords := ss.Meshes[mesh_id].Mesh.Verts.Get(vi)[0]
				average_border[i*3] += coords.X
				average_border[i*3+1] += coords.Y
				average_border[i*3+2] += coords.Z
			}
		}
		for i := 0; i < len(average_border); i++ {
			average_border[i] /= float64(len(mesh_ids))
		}

		// Update all instances of vertices in this border to their respective
		// averages.
		for _, mesh_id := range mesh_ids {
			for i, vi := range ss.Meshes[mesh_id].Borders[border_id] {
				ss.Meshes[mesh_id].Mesh.Verts.UpdateOne(
					vi,
					average_border[i*3],
					average_border[i*3+1],
					average_border[i*3+2],
				)
			}
		}
	}
	return
}
