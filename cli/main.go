package main

import (
	"errors"
	"fmt"
	"github.com/nat-n/piper"
	"github.com/nat-n/shapeset"
	"io"
	"os"
	"strconv"
	"strings"
)

/* Commands:
 * create
 * load
 * save
 * save-meshes
 * index-borders
 * simplify-borders
 * reload-vertices
 * create-region
 * center-and-scale
 */

func create(data interface{}, flags map[string]piper.Flag, args []string) (result interface{}, err error) {
	if _, verbose := flags["verbose"]; verbose {
		fmt.Println("Creating Shapeset")
	}

	meshes_dir := args[0]
	labels_path := args[1]

	var ss *shapeset.ShapeSet
	ss, err = shapeset.CreateNew(meshes_dir, labels_path)

	result = interface{}(ss)
	return
}

func load(data interface{}, flags map[string]piper.Flag, args []string) (result interface{}, err error) {
	if _, verbose := flags["verbose"]; verbose {
		fmt.Println("Loading ShapeSet")
	}
	// Load shapset from json file
	input_path := args[0]
	ss, err := shapeset.ReadFile(input_path)
	if err != nil {
		return
	}
	result = interface{}(ss)
	return
}

func save(data interface{}, flags map[string]piper.Flag, args []string) (result interface{}, err error) {
	if _, verbose := flags["verbose"]; verbose {
		fmt.Println("Saving ShapeSet to file")
	}
	// Serialize JSON and stream to a file
	ss := data.(*shapeset.ShapeSet)
	output_path := args[0]
	output_file, err := os.Create(output_path)
	defer output_file.Close()
	if err != nil {
		return
	}
	ss_writer := io.Writer(output_file)
	err = ss.Save(&ss_writer)
	if err != nil {
		return
	}

	result = data
	return
}

func save_meshes(data interface{}, flags map[string]piper.Flag, args []string) (result interface{}, err error) {
	if _, verbose := flags["verbose"]; verbose {
		fmt.Println("Saving meshes to directory")
	}
	ss := data.(*shapeset.ShapeSet)
	meshes_dir := args[0]

	// ensure meshes_dir ends with a slash
	if meshes_dir[len(meshes_dir)-1] != 47 {
		meshes_dir += "/"
	}

	// ensure meshes_dir is a directory
	path_stat, err := os.Stat(meshes_dir)
	if os.IsNotExist(err) || !path_stat.Mode().IsDir() {
		panic(errors.New("Provided path for saving meshes is not a directory"))
	}

	// write meshes as obj files into the given directory
	for _, mw := range ss.Meshes {
		new_mesh_filename := meshes_dir + mw.Mesh.Name + ".obj"
		mw.Mesh.WriteOBJFile(new_mesh_filename)
	}

	result = data
	return
}

func index_borders(data interface{}, flags map[string]piper.Flag, args []string) (result interface{}, err error) {
	if _, verbose := flags["verbose"]; verbose {
		fmt.Println("Indexing borders")
	}
	deref := data
	ss := deref.(*shapeset.ShapeSet)
	err = ss.IndexBorders()
	if err != nil {
		return
	}
	result = interface{}(ss)
	return
}

func simplify_borders(data interface{}, flags map[string]piper.Flag, args []string) (result interface{}, err error) {
	if _, verbose := flags["verbose"]; verbose {
		fmt.Println("Simplifying borders")
	}
	ss := data.(*shapeset.ShapeSet)

	const error_threshold = 1.0
	const aggressiveness = 0.75
	const forgiveness = 10

	ss.SimplifyBorders(error_threshold, aggressiveness, forgiveness)
	result = interface{}(ss)
	return
}

func reload_vertices(data interface{}, flags map[string]piper.Flag, args []string) (result interface{}, err error) {
	if _, verbose := flags["verbose"]; verbose {
		fmt.Println("Reloading mesh vertices")
	}
	ss := data.(*shapeset.ShapeSet)
	meshes_dir := args[0]

	ss.ReloadVertices(meshes_dir)

	result = interface{}(ss)
	return
}

func create_region(data interface{}, flags map[string]piper.Flag, args []string) (result interface{}, err error) {
	if _, verbose := flags["verbose"]; verbose {
		fmt.Println("Creating region " + args[0])
	}
	ss := data.(*shapeset.ShapeSet)
	shapes_str := args[0]
	mesh_path := args[1]

	// parse list of int shape ids from first argument
	string_segments := strings.Split(shapes_str, ",")
	shape_ids := make([]int, 0, len(string_segments))
	var num int
	for _, seg := range string_segments {
		num, err = strconv.Atoi(seg)
		if err != nil {
			panic(errors.New("Invalid region definition: Couldn't parse int from: " +
				seg))
		}
		shape_ids = append(shape_ids, num)
	}

	m, _ := ss.ComposeRegion(shape_ids...)
	obj_file, err := os.Create(mesh_path)
	if err != nil {
		return
	}
	m.WriteOBJ(obj_file)

	result = data
	return
}

func center_and_scale(data interface{}, flags map[string]piper.Flag, args []string) (result interface{}, err error) {
	if _, verbose := flags["verbose"]; verbose {
		fmt.Println("Centering and Scaling")
	}
	bb_max_dimension, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		return
	}

	ss := data.(*shapeset.ShapeSet)
	ss.ScaleAndCenter(bb_max_dimension)
	result = interface{}(ss)
	return
}

func main() {
	cli := piper.CLIApp{
		Name:        "shapeset",
		Description: "creates and processes shapesets",
	}

	cli.RegisterFlag(piper.Flag{
		Name:        "verbose",
		Symbol:      "v",
		Description: "Verbose mode",
	})

	cli.RegisterCommand(piper.Command{
		Name:        "create",
		Description: "create new shapeset from meshes and labels",
		Args:        []string{"meshes directory", "labels file"},
		Task:        create,
	})

	cli.RegisterCommand(piper.Command{
		Name:        "load",
		Description: "load shapeset from file",
		Args:        []string{"shapeset file"},
		Task:        load,
	})

	cli.RegisterCommand(piper.Command{
		Name:        "save",
		Description: "save shapeset to file",
		Args:        []string{"shapeset file"},
		Task:        save,
	})

	cli.RegisterCommand(piper.Command{
		Name:        "save-meshes",
		Description: "save meshes as obj files",
		Args:        []string{"meshes directory"},
		Task:        save_meshes,
	})

	cli.RegisterCommand(piper.Command{
		Name:        "index-borders",
		Description: "find mesh borders and create new shape set wide border index",
		Task:        index_borders,
	})

	cli.RegisterCommand(piper.Command{
		Name:        "simplify-borders",
		Description: "apply edge collapse simplification to all borders",
		Args:        []string{},
		Task:        simplify_borders,
	})

	cli.RegisterCommand(piper.Command{
		Name:        "reload-vertices",
		Description: "reload mesh vertex positions",
		Args:        []string{"meshes directory"},
		Task:        reload_vertices,
	})

	cli.RegisterCommand(piper.Command{
		Name: "create-region",
		Description: ("creates a mesh of a specified region as an obj file, " +
			"accepts shape ids as a comma seperated string of integers"),
		Args: []string{"region shape ids", "output obj file"},
		Task: create_region,
	})

	cli.RegisterCommand(piper.Command{
		Name: "center-and-scale",
		Description: ("transforms the whole shapeset so that its bounding box is " +
			"centered on the origin, and the extent of its largest dimension is " +
			"equal to the provied value"),
		Args: []string{"max bounding box dimension"},
		Task: center_and_scale,
	})

	err := cli.Run()

	if err != nil {
		fmt.Println(err)
		cli.PrintHelp()
	}
}
