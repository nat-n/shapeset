// go run github.com/nat-n/shapeset/sstool/sstool.go -im /Users/nat/Projects/rumesh_reboot/oct2abr/meshes/4_decimated_obj -l /Users/nat/Projects/rumesh_reboot/oct2abr/labels.json -index_borders -decimate_edges -om /Users/nat/Projects/rumesh_reboot/oct2abr/meshes/5_final_obj -o /Users/nat/Projects/rumesh_reboot/oct2abr/shape_set.json_fin

/*
 * Alternate argument structure:
 * -pipeline task-name:arguments,if,required, task-name:arguments,if,required, ...
 *
 * sstool -v -p create:[meshes:/path/to/meshesdir1 \
 *                      labels:/path/to/labels.json] \
 *              index_borders \
 *              save:/path/to/output1.json \
 *              decimate_edges \
 *              save_meshes:/path/to/meshesdir2 \
 *              save:/path/to/output2.json
 *
 * * brackets are whitespace sugar
 * * verbose mode prints a report for each stage with **** decorative lines between etc
 *
 * sstool -h
 * * print usage help
 *
 * * allows for stages to occur in specific order multiple times in the pipeline
 * * only state persisted between stages is the ss object... right?
 *
 * It seems this will require writing a custom args parser...
 * an alternative would be to accept a single sub command, OR a script as a json
 * file/piped string.
 *
 * Try reproduce this pattern? : http://www.thegeekstuff.com/2009/10/unix-sed-tutorial-how-to-execute-multiple-sed-commands/
 *
 */

// what if every stage in the whole ss pipeline had an input and output dir (with manifest file)?
// instead of sharing the one dir structure...

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/nat-n/gomesh/mesh"
	"github.com/nat-n/shapeset"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
)

/*
 * Example Usages:
 *
 * Create shapeset with borders from meshes and label files:
 * shapeset -m /path/to/meshes_dir -l /path/to/labels.json -o /path/to/output.shapeset.json
 *
 * Load shapeset, reload vertices of meshes, realign borders and write updated shapeset.
 * shapeset -i /path/to/input.shapeset.json -v /path/to/meshes_dir --realign-borders -o /path/to/output.shapeset.json
 *
 * Load shapeset, load updated (decimated) meshes, apply border decimation:
 * shapeset -i /path/to/input.shapeset.json -m /path/to/meshes_dir --decimate-borders -o /path/to/output.shapeset.json
 *
 */

func load_shape_set(input_path string) *shapeset.ShapeSet {
	ss, err := shapeset.ReadFile(input_path)
	if err != nil {
		panic(err)
	}
	return ss
}

func save_shape_set(output_path string, ss *shapeset.ShapeSet) {
	// Serialized JSON and stream to a file
	output_file, err := os.Create(output_path)
	defer output_file.Close()
	if err != nil {
		panic(err)
	}
	ss_writer := io.Writer(output_file)
	err = ss.Save(&ss_writer)
	if err != nil {
		panic(err)
	}
}

func create_shapeset(meshes_path, labels_path string) *shapeset.ShapeSet {
	var err error

	// ensure meshes_path ends with a slash
	if meshes_path[len(meshes_path)-1] != 47 {
		meshes_path += "/"
	}

	// Load labels file
	labels := make(map[string]string, 0)
	var labels_temp interface{}
	label_data, err := ioutil.ReadFile(labels_path)
	if err != nil {
		panic(err)
	}

	json.Unmarshal(label_data, &labels_temp)

	for key, val := range labels_temp.(map[string]interface{}) {
		labels[key] = val.(string)
	}

	// Load all meshes with the expected filename pattern from the directory at
	// meshes path.
	meshes := make([]*mesh.Mesh, 0)
	files, _ := ioutil.ReadDir(meshes_path)
	for _, f := range files {
		r, _ := regexp.Compile(`^(\d+-\d+).obj$`)
		if r.MatchString(f.Name()) == true {
			mesh_file_path := meshes_path + f.Name()
			m, err := mesh.ReadOBJFile(mesh_file_path)
			m.Name = string(r.FindSubmatch([]byte(f.Name()))[1])
			if err != nil {
				panic(err)
			}
			meshes = append(meshes, m)
		}
	}

	// Compose shapeset
	return shapeset.New("New ShapeSet", meshes, labels)
}

func reload_vertices(meshes_path string, ss *shapeset.ShapeSet) {
	// ensure meshes_path ends with a slash
	if meshes_path[len(meshes_path)-1] != 47 {
		meshes_path += "/"
	}

	// Reload vertices for all meshes in the shapeset
	for _, mw := range ss.Meshes {
		mesh_file_path := meshes_path + mw.Mesh.Name + ".obj"
		if _, err := os.Stat(mesh_file_path); os.IsNotExist(err) {
			panic(err)
		}

		// Empty Vertex positions from the mesh before reloading from the mesh file.
		mw.Mesh.Verts.Buffer = mw.Mesh.Verts.Buffer[:0]

		// scan mesh file line by line to find vertex definitions
		file, err := os.Open(mesh_file_path)
		if err != nil {
			panic(err)
		}

		// setup for parsing
		var (
			line  string
			words []string
			new_x float64
			new_y float64
			new_z float64
			err_x error
			err_y error
			err_z error
		)
		line_no := -1

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line_no++
			// trim leading and trailing whitespace
			line = strings.TrimSpace(scanner.Text())
			// firstly discard anything on this line after a #
			if comment_start := strings.Index(line, "#"); comment_start >= 0 {
				line = line[:comment_start]
			}
			// ignore empty lines
			if len(line) == 0 {
				continue
			}
			words = strings.Fields(line)
			if words[0] == "v" {
				new_x, err_x = strconv.ParseFloat(words[1], 64)
				new_y, err_y = strconv.ParseFloat(words[2], 64)
				new_z, err_z = strconv.ParseFloat(words[3], 64)
				if err_x != nil || err_y != nil || err_z != nil {
					panic(errors.New(
						"Error parsing OBJ file on line: " +
							strconv.Itoa(line_no)))
				}
			}
			mw.Mesh.Verts.Append(new_x, new_y, new_z)
		}

		if err := scanner.Err(); err != nil {
			panic(err)
		}

		file.Close()
	}

}

// Should this be moved onto the ShapeSet type?
func save_meshes(meshes_path string, ss *shapeset.ShapeSet) {
	// ensure meshes_path ends with a slash
	if meshes_path[len(meshes_path)-1] != 47 {
		meshes_path += "/"
	}

	// ensure meshes_path is a directory
	path_stat, err := os.Stat(meshes_path)
	if os.IsNotExist(err) || !path_stat.Mode().IsDir() {
		panic(errors.New("Provided path for saving meshes is not a directory"))
	}

	// write meshes as obj files into the given directory
	for _, mw := range ss.Meshes {
		new_mesh_filename := meshes_path + mw.Mesh.Name + ".obj"
		mw.Mesh.WriteOBJFile(new_mesh_filename)
	}
}

func reload_meshes(meshes_dir string, ss *shapeset.ShapeSet) {
	// ensure meshes_dir ends with a slash
	if meshes_dir[len(meshes_dir)-1] != 47 {
		meshes_dir += "/"
	}

	// ensure meshes_dir is a directory
	path_stat, err := os.Stat(meshes_dir)
	if os.IsNotExist(err) || !path_stat.Mode().IsDir() {
		panic(errors.New("Provided path for reloading meshes is not a directory"))
	}

	// Reload meshes from meshes_dir, and overwrite mesh references in ss with new
	// mesh instances.
	for _, mw := range ss.Meshes {
		mesh_name := mw.Mesh.Name
		mesh_filename := meshes_dir + mesh_name + ".obj"
		mw.Mesh, err = mesh.ReadOBJFile(mesh_filename)
		mw.Mesh.Name = mesh_name
	}
}

func simplify_borders(ss *shapeset.ShapeSet) {
	ss.SimplifyBorders()
}

func main() {
	input_path := flag.String("i", "", "input shape set file")
	output_path := flag.String("o", "", "output shape set file")

	input_meshes := flag.String("im", "", "directory to reload meshes from")
	labels_path := flag.String("l", "", "input labels file")
	mesh_verts := flag.String("vm", "", "directory to reload mesh vertices from")
	output_meshes := flag.String("om", "", "directory to save meshes to")

	realign_borders := flag.Bool("realign_borders", false, "Realign borders")
	index_borders := flag.Bool("index_borders", false, "Index borders")
	decimate_edges := flag.Bool("decimate_edges", false, "Perform decimation")

	flag.Parse()

	// fmt.Println("input_path:", *input_path)
	// fmt.Println("output:", *output_path)

	// fmt.Println("input_meshes:", *input_meshes)
	// fmt.Println("labels_path:", *labels_path)
	// fmt.Println("mesh_verts:", *mesh_verts)
	// fmt.Println("output_meshes:", *output_meshes)

	// fmt.Println("realign_borders:", *realign_borders)
	// fmt.Println("index_borders:", *index_borders)
	// fmt.Println("decimate_edges:", *decimate_edges)

	// fmt.Println("tail:", flag.Args())

	var ss *shapeset.ShapeSet
	var err error

	// Create shapeset from meshes and labels or load shapeset file
	if len(*labels_path) > 0 && len(*input_meshes) > 0 {
		ss = create_shapeset(*input_meshes, *labels_path)
	} else if len(*input_path) > 0 {
		ss, err = shapeset.ReadFile(*input_path)
		if err != nil {
			panic(err)
		}
	} else {
		fmt.Println("Error: No input data provided")
		flag.PrintDefaults()
		return
	}

	// If the shape set was loaded from a shapeset file and an input meshes
	// directory is provided
	if len(*input_path) > 0 && len(*input_meshes) > 0 && len(*labels_path) == 0 {
		reload_meshes(*input_meshes, ss)
	}

	// if mesh_verts is set then reload vertex positions into the current
	// shape set from meshes in the given directory. This of course assumes that
	// the number and ordering of vertices is unchanged.
	if len(*mesh_verts) > 0 {
		reload_vertices(*mesh_verts, ss)
	}

	// if index_borders set then do so
	if *index_borders {
		ss.IndexBorders()
		// fmt.Println(ss.VerifyBorders())
	}

	// if realign_borders set then do so
	if *realign_borders {
		ss.RealignBorders()
		// fmt.Println(ss.VerifyBorders())
	}

	// if decimate_edges set then do so
	if *decimate_edges {
		simplify_borders(ss)
	}

	// if output_meshes set then do so
	if len(*output_meshes) > 0 {
		save_meshes(*output_meshes, ss)
	}

	// If output path given then write resulting shapeset to file
	if len(*output_path) > 0 {
		err = ss.WriteFile(*output_path)
		if err != nil {
			panic(err)
		}
	}
}
