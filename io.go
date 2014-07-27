package shapeset

import (
	"encoding/json"
	"errors"
	"github.com/nat-n/gomesh/cuboid"
	"github.com/nat-n/gomesh/mesh"
	"os"
	"strconv"
	"strings"
)

type ShapeSet struct {
	Name         string
	BordersIndex map[string]string
	Meshes       map[string]MeshWrapper
	Shapes       map[int]string
}

type MeshWrapper struct {
	Mesh        *mesh.Mesh
	Borders     map[string][]int
	BoundingBox *cuboid.Cuboid
}

type mesh_temp_schema struct {
	Name    string            `json:"name"`
	Verts   string            `json:"verts"`
	Norms   string            `json:"norms"`
	Faces   string            `json:"faces"`
	Borders map[string]string `json:"borders"`
}

type shape_set_temp_schema struct {
	Name   string              `json:"name"`
	Shapes map[string]string   `json:"shapes"`
	Meshes []*mesh_temp_schema `json:"meshes"`
}

func (ss *ShapeSet) Load(ss_path string) (err error) {
	// Open file
	file, _ := os.Open(ss_path)
	if err != nil {
		err = errors.New("Could not open file: " + ss_path)
		return
	}

	// Parse json and load into temporary types
	temp_data := new(shape_set_temp_schema)
	err = json.NewDecoder(file).Decode(temp_data)
	if err != nil {
		err = errors.New("Could not parse json from: " + ss_path)
		return
	}

	// Unpack data into a Meshes and ShapeSet
	ss.Name = temp_data.Name

	ss.Shapes = make(map[int]string)
	for k, shape_label := range temp_data.Shapes {
		shape_value, _ := strconv.Atoi(k)
		ss.Shapes[shape_value] = shape_label
	}

	ss.Meshes = make(map[string]MeshWrapper)
	var verts, norms []float64
	var faces []int

	for _, mesh_data := range temp_data.Meshes {
		m := mesh.New(mesh_data.Name)
		verts, err = parseCSFloats(mesh_data.Verts)
		if err != nil {
			err = errors.New("Could not parse vertices for: " + m.Name)
			return
		}
		m.Verts.Append(verts...)
		norms, err = parseCSFloats(mesh_data.Norms)
		if err != nil {
			err = errors.New("Could not parse normals for: " + m.Name)
			return
		}
		m.Norms.Append(norms...)
		faces, err = parseCSInts(mesh_data.Faces)
		if err != nil {
			err = errors.New("Could not parse faces for: " + m.Name)
			return
		}
		m.Faces.Append(faces...)
		b := make(map[string][]int)

		for border_id, border_indices := range mesh_data.Borders {
			b[border_id], err = parseCSInts(border_indices)
			if err != nil {
				err = errors.New(
					"Could not parse border indices for border " +
						border_id + ", in mesh " + m.Name)
				return
			}
		}

		bb := m.BoundingBox()
		ss.Meshes[m.Name] = MeshWrapper{m, b, bb}
	}

	return nil
}

func (ss *ShapeSet) Save(ss_path string) (err error) {
	// structure data for serialization
	temp_data := shape_set_temp_schema{
		Name:   ss.Name,
		Shapes: make(map[string]string),
		Meshes: make([]*mesh_temp_schema, 0, len(ss.Meshes)),
	}
	for shape_value, shape_name := range ss.Shapes {
		temp_data.Shapes[strconv.Itoa(shape_value)] = shape_name
	}
	for mesh_name, mesh_data := range ss.Meshes {
		temp_mesh_data := mesh_temp_schema{
			Name:    mesh_name,
			Verts:   mesh_data.Mesh.Verts.ToString(),
			Norms:   mesh_data.Mesh.Norms.ToString(),
			Faces:   mesh_data.Mesh.Faces.ToString(),
			Borders: make(map[string]string),
		}
		for border_id, border_verts := range mesh_data.Borders {
			// stringify border vertex indices
			stringInts := make([]string, len(border_verts), len(border_verts))
			for i := 0; i < len(border_verts); i++ {
				stringInts[i] = strconv.FormatInt(int64(border_verts[i]), 10)
			}
			temp_mesh_data.Borders[string(border_id)] = strings.Join(stringInts, ",")
		}
		temp_data.Meshes = append(temp_data.Meshes, &temp_mesh_data)
	}

	// Serialized JSON and stream to a file
	f, err := os.Create(ss_path)
	if err != nil {
		err = errors.New("Could not create output file at: " + ss_path)
		return
	}
	err = json.NewEncoder(f).Encode(&temp_data)
	if err != nil {
		err = errors.New("Could not encode json for: " + ss.Name)
		return
	}

	return nil
}
