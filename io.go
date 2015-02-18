package shapeset

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nat-n/gomesh/mesh"
	"io"
	"os"
	"strconv"
	"strings"
)

type meshParseSchema struct {
	Name    string            `json:"name"`
	Verts   string            `json:"verts"`
	Norms   string            `json:"norms"`
	Faces   string            `json:"faces"`
	Borders map[string]string `json:"borders"`
}

type shapeSetParseSchema struct {
	Name   string             `json:"name"`
	Shapes map[string]string  `json:"shapes"`
	Meshes []*meshParseSchema `json:"meshes"`
}

func Load(ss_reader *io.Reader) (ss *ShapeSet, err error) {
	ss = &ShapeSet{}
	// Parse json from reader and load into temporary types
	temp_data := new(shapeSetParseSchema)
	err = json.NewDecoder(*ss_reader).Decode(temp_data)
	if err != nil {
		err = errors.New("Could not parse json from ss_reader")
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
		// Vertex normals are optional
		if len(mesh_data.Norms) > 0 {
			fmt.Println(mesh_data.Norms)
			norms, err = parseCSFloats(mesh_data.Norms)
			if err != nil {
				err = errors.New("Could not parse normals for: " + m.Name)
				return
			}
			m.Norms.Append(norms...)
		}
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

	// Build BordersIndex
	ss.BordersIndex = make(map[string]string)
	// first track which meshes participate in each border
	participants := make(map[string][]string)
	for _, mw := range ss.Meshes {
		for border_id, _ := range mw.Borders {
			if _, exists := participants[border_id]; !exists {
				participants[border_id] = make([]string, 0)
			}
			participants[border_id] = append(
				participants[border_id],
				mw.Mesh.Name,
			)
		}
	}

	// for border_id, border_participants := range participants {
	// 	lengths := make([]int, 0)
	// 	for _, mesh_id := range border_participants {
	// 		lengths = append(lengths, len(ss.Meshes[mesh_id].Borders[border_id]))
	// 	}
	// 	fmt.Println(border_participants, lengths)
	// 	if len(border_participants) != len(lengths) {
	// 		panic("foo")
	// 	}
	// }

	for border_id, border_participants := range participants {
		border_desc := strings.Join(border_participants, "_")
		ss.BordersIndex[border_desc] = border_id
	}

	return
}

func (ss *ShapeSet) Save(ss_writer *io.Writer) (err error) {
	// structure data for serialization
	temp_data := shapeSetParseSchema{
		Name:   ss.Name,
		Shapes: make(map[string]string),
		Meshes: make([]*meshParseSchema, 0, len(ss.Meshes)),
	}
	for shape_value, shape_name := range ss.Shapes {
		temp_data.Shapes[strconv.Itoa(shape_value)] = shape_name
	}
	for mesh_name, mesh_data := range ss.Meshes {
		temp_mesh_data := meshParseSchema{
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

	err = json.NewEncoder(*ss_writer).Encode(&temp_data)
	if err != nil {
		err = errors.New("Could not encode json for: " + ss.Name)
		return
	}

	return nil
}

func ReadFile(ss_file_path string) (ss *ShapeSet, err error) {
	// open file
	input_file, err := os.Open(ss_file_path)
	if err != nil {
		return
	}
	defer input_file.Close()

	// read from file
	ss_reader := io.Reader(input_file)
	ss, err = Load(&ss_reader)

	return
}

func (ss *ShapeSet) WriteFile(ss_file_path string) (err error) {
	// Serialized JSON and stream to a file
	output_file, err := os.Create(ss_file_path)
	if err != nil {
		return
	}
	defer output_file.Close()
	ss_writer := io.Writer(output_file)
	ss.Save(&ss_writer)

	return
}

// Write Meshes
// func (ss *ShapeSet) WriteMeshes(ss_file_path string) (err error) {

// }
