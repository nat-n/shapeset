package shapeset

import (
	"bufio"
	"encoding/json"
	"errors"
	"github.com/nat-n/geom"
	gomesh "github.com/nat-n/gomesh/mesh"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"sort"
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
	// Parse json from reader and load with temporary types
	parsed_data := new(shapeSetParseSchema)
	err = json.NewDecoder(*ss_reader).Decode(parsed_data)
	if err != nil {
		err = errors.New("Could not parse json from ss_reader")
		return
	}

	// for building up partial border info from meshes
	border_tracker := make(map[BorderId]map[MeshId][]*Vertex)

	meshesBuffer := make([]*Mesh, 0)
	for _, mesh_data := range parsed_data.Meshes {
		var verts, norms []float64
		var faces []int
		mesh_name := mesh_data.Name
		m := NewMesh(mesh_name)

		// load vertices from parsed data
		verts, err = parseCSFloats(mesh_data.Verts)
		vertexBuffer := make([]gomesh.VertexI, 0, len(verts)/3)
		if err != nil {
			err = errors.New("Could not parse vertices for: " + mesh_name)
			return
		}
		for i := 0; i < len(verts); i += 3 {
			vertexBuffer = append(vertexBuffer, &Vertex{Vertex: gomesh.Vertex{
				Vec3:   geom.Vec3{verts[i], verts[i+1], verts[i+2]},
				Meshes: make(map[gomesh.Mesh]int),
			}})
		}

		// Vertex normals are optional
		if len(mesh_data.Norms) > 0 {
			norms, err = parseCSFloats(mesh_data.Norms)
			if err != nil {
				err = errors.New("Could not parse normals for: " + mesh_name)
				return
			}
			if len(norms) != 0 && len(norms) != len(verts) {
				err = errors.New("Malformed Mesh (vertices/normals mismatch): " + mesh_name)
				return
			}
			for i := 0; i < len(norms); i += 3 {
				vertexBuffer[i/3].SetNormal(&geom.Vec3{norms[i], norms[i+1], norms[i+2]})
			}
		}

		edges := make(map[gomesh.VertexPair]*Edge)

		// load faces from parsed data
		faces, err = parseCSInts(mesh_data.Faces)
		faceBuffer := make([]gomesh.FaceI, 0, len(faces)/3)
		if err != nil {
			err = errors.New("Could not parse faces for: " + mesh_name)
			return
		}
		for i := 0; i < len(faces); i += 3 {
			// create new face from vertex triple
			vA := vertexBuffer[faces[i]]
			vB := vertexBuffer[faces[i+1]]
			vC := vertexBuffer[faces[i+2]]
			new_face := &Face{Face: gomesh.Face{Vertices: [3]gomesh.VertexI{vA, vB, vC}}}

			// reference face from vertices
			vA.AddFace(new_face)
			vB.AddFace(new_face)
			vC.AddFace(new_face)
			faceBuffer = append(faceBuffer, new_face)

			// find/create edges of new_face
			e0 := gomesh.MakeVertexPair(vA, vB)
			e1 := gomesh.MakeVertexPair(vB, vC)
			e2 := gomesh.MakeVertexPair(vC, vA)
			newPairs := [3]gomesh.VertexPair{e0, e1, e2}
			for i, vp := range newPairs {
				var f_edge *Edge
				if e, exists := edges[vp]; exists {
					f_edge = e
				} else {
					f_edge = &Edge{VertexPair: vp}
					f_edge.Vertex1().AddEdge(f_edge)
					f_edge.Vertex2().AddEdge(f_edge)
					edges[vp] = f_edge
				}
				f_edge.AddFace(new_face)
				new_face.Edges[i] = f_edge
			}
		}

		for border_id, border_indices := range mesh_data.Borders {
			var vert_indices []int
			vert_indices, err = parseCSInts(border_indices)
			if err != nil {
				err = errors.New(
					"Could not parse border for mesh " + m.GetName() +
						" indices for border " + border_id + ", in mesh " + mesh_name)
				return
			}

			bverts := make([]*Vertex, 0)
			for _, vi := range vert_indices {
				bverts = append(bverts, vertexBuffer[vi].(*Vertex))
			}
			bid, err := BorderIdFromString(border_id)
			if err != nil {
				panic(err.Error())
			}
			mid := MeshIdFromString(mesh_name)
			if _, exists := border_tracker[bid]; !exists {
				border_tracker[bid] = make(map[MeshId][]*Vertex)
			}
			border_tracker[bid][mid] = bverts
		}

		m.Vertices.Append(vertexBuffer...)
		m.Faces.Append(faceBuffer...)
		m.ReindexVerticesAndFaces()
		m.BoundingBox = m.Mesh.BoundingBox()
		meshesBuffer = append(meshesBuffer, m)
	}

	// Create ShapeSet
	ss = New(parsed_data.Name, parsed_data.Shapes, meshesBuffer)

	// merge border vertices
	for border_id, mesh_borders := range border_tracker {
		mesh_ids := ByMeshIdPrecedence{}
		for mesh_id, _ := range mesh_borders {
			mesh_ids = append(mesh_ids, mesh_id)
		}
		sort.Sort(mesh_ids)

		first_mesh_vertices := mesh_borders[mesh_ids[0]]
		for _, mesh_id := range mesh_ids[1:] {
			secondary_mesh_vertices := mesh_borders[mesh_id]
			for i := 0; i < len(first_mesh_vertices); i++ {
				gomesh.MergeSharedVertices(
					first_mesh_vertices[i],
					secondary_mesh_vertices[i])
			}
		}

		_, err = ss.BordersIndex.LoadBorder(
			border_id,
			[]MeshId(mesh_ids),
			first_mesh_vertices,
		)
		if err != nil {
			return
		}
	}

	ss.BordersIndex.indexBorderEdges()

	return
}

func (ss *ShapeSet) Save(ss_writer *io.Writer) (err error) {
	// structure data for serialization
	parsed_data := shapeSetParseSchema{
		Name:   ss.Name,
		Shapes: make(map[string]string),
		Meshes: make([]*meshParseSchema, 0, len(ss.Meshes)),
	}
	for shape_id, shape_name := range ss.Shapes {
		parsed_data.Shapes[shape_id.ToString()] = shape_name
	}

	for mesh_name, m := range ss.Meshes {
		m.ReindexVerticesAndFaces()
		m.Vertices.EnsureNormals()
		temp_mesh_data := meshParseSchema{
			Name:    mesh_name.ToString(),
			Verts:   m.Vertices.PositionsAsCSV(),
			Norms:   m.Vertices.NormalsAsCSV(),
			Faces:   m.Faces.IndicesAsCSV(),
			Borders: make(map[string]string),
		}
		for border_id, border := range m.Borders {
			// stringify border vertex indices
			stringInts := make([]string, border.Len(), border.Len())
			for i := 0; i < border.Len(); i++ {
				bvi := border.Vertices[i].GetLocationInMesh(m)
				stringInts[i] = strconv.FormatInt(int64(bvi), 10)
			}
			temp_mesh_data.Borders[border_id.ToString()] = strings.Join(stringInts, ",")
		}
		parsed_data.Meshes = append(parsed_data.Meshes, &temp_mesh_data)
	}

	err = json.NewEncoder(*ss_writer).Encode(&parsed_data)
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

/* Create a new shapeset from a json labels file and a directory of meshes.
 */
func CreateNew(meshes_dir, labels_path string) (ss *ShapeSet, err error) {
	// Ensure meshes_dir ends with a slash
	if meshes_dir[len(meshes_dir)-1] != 47 {
		meshes_dir += "/"
	}

	// Attempt to read given paths
	var label_data []byte
	label_data, err = ioutil.ReadFile(labels_path)
	if err != nil {
		return
	}
	var files []os.FileInfo
	files, err = ioutil.ReadDir(meshes_dir)
	if err != nil {
		return
	}

	// Load labels from json file
	labels := make(map[string]string, 0)
	var labels_temp interface{}
	json.Unmarshal(label_data, &labels_temp)
	for key, val := range labels_temp.(map[string]interface{}) {
		labels[key] = val.(string)
	}

	// Load all meshes with the expected filename pattern (.obj) from the
	// directory at meshes path.
	meshes := make([]*Mesh, 0)
	for _, f := range files {
		r, _ := regexp.Compile(`^(\d+-\d+).obj$`)
		if r.MatchString(f.Name()) == true {
			mesh_file_path := meshes_dir + f.Name()
			var m *gomesh.Mesh
			m, err = gomesh.ReadOBJFile(mesh_file_path)
			if err != nil {
				return
			}
			m.Name = string(r.FindSubmatch([]byte(f.Name()))[1])
			meshes = append(meshes, WrapMesh(m))
		}
	}

	// Compose shapeset
	ss = New("New ShapeSet", labels, meshes)
	return
}

/* Loads a directory of meshes and replaces each location of a vertex currently
 * in the shapeset with the location of the corresponding vertex (by index) in
 * the identically named mesh file. The set of meshes and their topology is
 * assumed to be the same as is present in the shapeset.
 */
func (ss *ShapeSet) ReloadVertices(meshes_dir string) {
	// ensure meshes_dir ends with a slash
	if meshes_dir[len(meshes_dir)-1] != 47 {
		meshes_dir += "/"
	}

	// ensure meshes_dir is a directory
	path_stat, err := os.Stat(meshes_dir)
	if os.IsNotExist(err) || !path_stat.Mode().IsDir() {
		panic(errors.New("Provided path for reloading meshes is not a directory"))
	}

	// collect border vertex positions to be averaged at the end
	borders_vertices := make(map[*Vertex][]geom.Vec3I)

	// reload vertices for all meshes in the shapeset
	for _, m := range ss.Meshes {
		mesh_file_path := meshes_dir + m.Name + ".obj"
		if _, err = os.Stat(mesh_file_path); os.IsNotExist(err) {
			return
		}

		// scan mesh file line by line to find vertex definitions
		var file *os.File
		file, err = os.Open(mesh_file_path)
		if err != nil {
			return
		}

		// setup for parsing
		index := 0
		line_no := -1
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line_no++
			// trim leading and trailing whitespace
			line := strings.TrimSpace(scanner.Text())
			// discard anything on this line after a #
			if comment_start := strings.Index(line, "#"); comment_start >= 0 {
				line = line[:comment_start]
			}
			// ignore empty lines
			if len(line) == 0 {
				continue
			}

			var new_vec geom.Vec3
			words := strings.Fields(line)
			if words[0] == "v" {
				new_x, err_x := strconv.ParseFloat(words[1], 64)
				new_y, err_y := strconv.ParseFloat(words[2], 64)
				new_z, err_z := strconv.ParseFloat(words[3], 64)
				if err_x != nil || err_y != nil || err_z != nil {
					panic(errors.New(
						"Error parsing OBJ file on line: " +
							strconv.Itoa(line_no)))
				}
				new_vec = geom.Vec3{new_x, new_y, new_z}
			} else {
				// ignore lines that don't define a vertex
				continue
			}

			v := m.Vertices.Get(index)[0]
			vert := v.(*Vertex)
			if vert.IsShared() {
				// add border vertices, we'll then divide by number_of_meshes+1 to get
				// the mean
				borders_vertices[vert] = append(borders_vertices[vert], &new_vec)
			} else {
				vert.SetX(new_vec.X)
				vert.SetY(new_vec.Y)
				vert.SetZ(new_vec.Z)
			}
			// only incremented after parsing lines with a vertex
			index++
		}

		if err = scanner.Err(); err != nil {
			return
		}

		file.Close()
	}

	// Calculate border vertex positions as mean across meshes
	ss.BordersIndex.Each(func(border *Border) {
		for _, v := range border.Vertices {
			mean_border_vec := borders_vertices[v][0].Mean(borders_vertices[v][1:]...)
			v.X = mean_border_vec.X
			v.Y = mean_border_vec.Y
			v.Z = mean_border_vec.Z
		}
	})
	return
}
