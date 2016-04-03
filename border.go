package shapeset

import (
	"errors"
	"sort"
	"strconv"
	"strings"
)

type Border struct {
	Id       BorderId
	MeshIds  []MeshId
	Vertices []*Vertex
	Edges    []*Edge
	shapeSet *ShapeSet
}

func (b *Border) Description() BorderDescription {
	return BorderDescriptionFromMeshIds(b.MeshIds)
}

func (b *Border) Len() int {
	return len(b.Vertices)
}

func (b *Border) EachMesh(cb func(*Mesh)) {
	for _, mesh_id := range b.MeshIds {
		cb(b.shapeSet.Meshes[mesh_id])
	}
}

func (b *Border) RemoveEdge(e1 *Edge) {
	for i, e2 := range b.Edges {
		if e1 == e2 {
			b.Edges = append(b.Edges[:i], b.Edges[i+1:]...)
			return
		}
	}

	panic("dind't find edge in border to remove")
}
func (b *Border) RemoveVertex(v1 *Vertex) {
	for i, v2 := range b.Vertices {
		if v1 == v2 {
			b.Vertices = append(b.Vertices[:i], b.Vertices[i+1:]...)
			return
		}
	}

	panic("dind't find vertex in border to remove")
}

type BorderId int

func BorderIdFromString(s string) (bid BorderId, err error) {
	border_num, e := strconv.Atoi(s)
	if e != nil {
		err = errors.New("Couldn't parse border id from: " + s)
		return
	}
	bid = BorderId(border_num)
	return
}

func (b *BorderId) ToString() string {
	return strconv.Itoa(int(*b))
}

type BorderDescription struct {
	serial string
}

func (b *BorderDescription) ToString() string {
	return b.serial
}

func (b *BorderDescription) ToMeshIds() []MeshId {
	parts := strings.Split(b.serial, "_")
	mesh_ids := make([]MeshId, 0)
	for _, part := range parts {
		mesh_ids = append(mesh_ids, MeshIdFromString(part))
	}
	return mesh_ids
}

func BorderDescriptionFromMeshIds(mesh_ids []MeshId) BorderDescription {
	// assumes no duplicates in mesh_ids
	sort.Sort(ByMeshIdPrecedence(mesh_ids))
	str := ""
	for _, mesh_id := range mesh_ids {
		str += "_" + mesh_id.ToString()
	}
	return BorderDescription{strings.TrimLeft(str, "_")}
}

func BorderDescriptionFromString(s string) BorderDescription {
	// decompose and recompose string to ensure validity
	parts := strings.Split(s, "_")
	mesh_ids := make([]MeshId, 0)
	for _, part := range parts {
		mesh_ids = append(mesh_ids, MeshIdFromString(part))
	}
	return BorderDescriptionFromMeshIds(mesh_ids)
}

type BorderIndex struct {
	shapeSet     *ShapeSet
	counter      int
	borderById   map[BorderId]*Border
	borderByDesc map[BorderDescription]*Border
}

func (bi *BorderIndex) Each(cb func(*Border)) {
	for _, b := range bi.borderByDesc {
		cb(b)
	}
}

func (bi *BorderIndex) BorderFor(border_desc_or_id interface{}) (b *Border) {
	if border_desc, ok := border_desc_or_id.(BorderDescription); ok {
		if border, exists := bi.borderByDesc[border_desc]; exists {
			b = border
			return
		}
	} else if border_id, ok := border_desc_or_id.(BorderId); ok {
		if border, exists := bi.borderById[border_id]; exists {
			b = border
			return
		}
	} else {
		panic("BorderFor accepts either a BorderDescription or a BorderId")
	}
	return
}

func (bi *BorderIndex) indexBorderEdges() {
	// Builds up Border.Edges for each border assuming border.Vertices is in order
	// Identify border edges as they will have accumulated duplicates
	// deduplicate these edges, combining faces, and associate them with the
	// border corresponding to the set of meshes from which they have faces.
	bi.Each(func(border *Border) { border.Edges = make([]*Edge, 0) })
	bi.Each(func(border *Border) {
		for _, v := range border.Vertices {
			v_neighbors := make(map[*Vertex][]*Edge)
			for _, e := range v.Edges {
				v_neighbors[e.Vertex1()] = append(v_neighbors[e.Vertex1()], e)
				v_neighbors[e.Vertex2()] = append(v_neighbors[e.Vertex2()], e)
			}
			delete(v_neighbors, v)

			for _, shared_edges := range v_neighbors {
				if len(shared_edges) < 3 {
					// ignore non duplicated edges
					// and also coincidental edges shared by two meshes, that aren't
					// actually on the boundary of either mesh
					continue
				}

				first_edge := shared_edges[0]
				first_edge.Merge(shared_edges[1:]...)

				// Collected mesh ids of faces of resulting merged edge
				mesh_ids_set := make(map[MeshId]bool)
				mesh_ids_slc := make([]MeshId, 0)
				for _, f := range first_edge.Faces {
					mesh_ids_set[MeshIdFromString(f.Mesh.GetName())] = true
				}
				for mesh_id, _ := range mesh_ids_set {
					mesh_ids_slc = append(mesh_ids_slc, mesh_id)
				}
				// should usually but not always equal border
				edge_border := bi.BorderFor(BorderDescriptionFromMeshIds(mesh_ids_slc))
				first_edge.Border = edge_border
				if edge_border == nil {
					// Given that it is possible for an edge to shared by a set of meshes
					// which is different from the set of meshes in which either one or
					// both of its vertices are shared between, it is possible to
					// encounter edges that are shared between vertices but for for which
					// there is no border, in the sense that there are no vertices shared
					// by the identical complete set of meshes.
					// These edges will simply be ignored, i.e. left unindexed.
					continue
				}
				edge_border.Edges = append(edge_border.Edges, first_edge)
			}
		}
	})
}

func (bi *BorderIndex) NewBorder(border_desc BorderDescription) (new_border *Border, err error) {
	if bi.BorderFor(border_desc) != nil {
		err = errors.New("Border already exists: " + border_desc.ToString())
		return
	}
	for bi.BorderFor(BorderId(bi.counter)) != nil {
		bi.counter += 1
	}
	new_border_id := BorderId(bi.counter)
	new_border = &Border{
		Id:       new_border_id,
		MeshIds:  border_desc.ToMeshIds(),
		shapeSet: bi.shapeSet,
	}

	// index new border by id and description
	bi.borderById[new_border_id] = new_border
	bi.borderByDesc[border_desc] = new_border

	// register border with affected meshes
	new_border.EachMesh(func(m *Mesh) {
		m.Borders[new_border_id] = new_border
	})

	return
}

func (bi *BorderIndex) LoadBorder(
	border_id BorderId,
	mesh_ids []MeshId,
	vertices []*Vertex,
) (new_border *Border, err error) {
	border_desc := BorderDescriptionFromMeshIds(mesh_ids)
	if bi.BorderFor(border_desc) != nil {
		err = errors.New("Border already exists: " + border_desc.ToString())
		return
	}
	if bi.BorderFor(border_id) != nil {
		err = errors.New("Border already exists with Id: " + border_id.ToString())
		return
	}
	new_border = &Border{
		Id:       border_id,
		MeshIds:  mesh_ids,
		Vertices: vertices,
		shapeSet: bi.shapeSet,
	}

	// index new border by id and description
	bi.borderById[border_id] = new_border
	bi.borderByDesc[BorderDescriptionFromMeshIds(mesh_ids)] = new_border

	// register border with affected vertices
	for _, v := range new_border.Vertices {
		v.Border = new_border
	}

	// register border with affected meshes
	new_border.EachMesh(func(m *Mesh) {
		m.Borders[border_id] = new_border
	})
	return
}

func (ss *ShapeSet) ResetBorders() {
	ss.BordersIndex = BorderIndex{
		shapeSet:     ss,
		counter:      1, // it's important that the first BorderId is 1 and not 0
		borderById:   make(map[BorderId]*Border),
		borderByDesc: make(map[BorderDescription]*Border),
	}
}
