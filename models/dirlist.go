package models

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"strconv"
	"strings"
	"time"

	. "github.com/maltegrosse/go-minio-list/log"
	"github.com/minio/minio-go/v6"
)

// all the variables from the config file
var Endpoint string
var PublicEndpoint string
var AccessKeyID string
var SecretAccessKey string
var UseSSL bool
var PublicUseSSL bool
var BucketName string
var DirectUrl string
var PublicUrl string
var MetaFilename string
var ReverseProxy bool

// DirectoryEntryType defines the type of a DirectoryEntry
type DirectoryEntryType uint

const (
	// DEFile indicates a File
	DEFile DirectoryEntryType = 0

	// DEFolder indicates a Folder
	DEFolder DirectoryEntryType = 1
)

// DirectoryEntry represents a File or Folder in a Directory Tree
type DirectoryEntry struct {
	// type
	Type DirectoryEntryType

	// filename
	Path []string
	Name string

	// timestamps
	ATime time.Time
	CTime time.Time
	MTime time.Time

	// size
	Size int64

	// TODO: other known attributes

	// children
	Files []*DirectoryEntry

	// HACK: meta.html content, if any
	Meta template.HTML
	// HACK: is meta.html - so that it can be excluded from listing
	IsMeta bool
}

// DirectoryEntry_New instantiates a new directory entry
func DirectoryEntry_New(_type DirectoryEntryType, path []string, name string, atime, ctime, mtime time.Time, size int64, files []*DirectoryEntry) *DirectoryEntry {
	return &DirectoryEntry{
		Type:  _type,
		Path:  path,
		Name:  name,
		ATime: atime,
		CTime: ctime,
		MTime: mtime,
		Size:  size,
		Files: files,
	}
}

type byPath []DirectoryEntry

func (e byPath) Len() int { return len(e) }
func (e byPath) Less(i, j int) bool {
	lcp := LongestCommonPrefix(e[i].Path, e[j].Path)
	if len(e[i].Path) == len(lcp) && len(lcp) == len(e[j].Path) {
		return false
	}

	// compare next level
	return strings.Compare(e[i].Path[len(lcp)], e[j].Path[len(lcp)]) < 0
}
func (e byPath) Swap(i, j int) { e[i], e[j] = e[j], e[i] }

// DirectoryEntry_NewFile instantiates a new directory entry representing a File
func DirectoryEntry_NewFile(path []string, name string, atime, ctime, mtime time.Time, size int64) *DirectoryEntry {
	return DirectoryEntry_New(DEFile, path, name, atime, ctime, mtime, size, nil)
}

// DirectoryEntry_NewFolder instantiates a new directory entry representing a Folder
func DirectoryEntry_NewFolder(path []string, name string, atime, ctime, mtime time.Time, capacity int) *DirectoryEntry {
	return DirectoryEntry_New(DEFolder, path, name, atime, ctime, mtime, 0, make([]*DirectoryEntry, capacity))
}

func DirectoryEntry_FromObject(object minio.ObjectInfo) *DirectoryEntry {
	var err error
	var path []string
	var name string
	var atime time.Time = object.LastModified
	var ctime time.Time = object.LastModified
	var mtime time.Time = object.LastModified

	// split path and filename
	// Log.Info("Have object key " + object.Key)
	path = strings.Split(object.Key, "/")
	name = path[len(path)-1]
	if len(path) == 1 {
		path = make([]string, 0)
	} else {
		path = path[0 : len(path)-1]
	}

	// parse metadata for filesystem attributes, if exists
	if object.UserMetadata != nil {
		if metadataentry, exists := object.UserMetadata["X-Amz-Meta-Mc-Attrs"]; exists {
			// split at '/'
			for _, attribute := range strings.Split(metadataentry, "/") {
				// expect key-value pairs seperated by ':'
				kv := strings.Split(attribute, ":")
				if len(kv) < 2 {
					// object has invalid metadata - can't continue :(
					break
				}

				// evaluate particular attributes
				switch kv[0] {
				case "atime":
					if v, err := strconv.ParseInt(kv[1], 10, 64); err == nil {
						atime = time.Unix(v, 0)
					}
				case "ctime":
					if v, err := strconv.ParseInt(kv[1], 10, 64); err == nil {
						ctime = time.Unix(v, 0)
					}
				case "mtime":
					if v, err := strconv.ParseInt(kv[1], 10, 64); err == nil {
						mtime = time.Unix(v, 0)
					}
				default: // list of attributes is inconclusive - ignore unknown
				}
			}

			if err != nil {
				// filesystem attributes metadata corrupt - revert to object info
				atime, ctime, mtime = object.LastModified, object.LastModified, object.LastModified
			}
		}
	}

	return DirectoryEntry_NewFile(path, name, atime, ctime, mtime, object.Size)
}

// BuildDirectoryTree creates the recursive tree structure from a slice of DirectoryEntry
func BuildDirectoryTree(files []*DirectoryEntry) *DirectoryEntry {
	var level int = 0
	var root *DirectoryEntry
	var stack []*DirectoryEntry
	var stacks []string

	// create directory root node
	root = DirectoryEntry_NewFolder(make([]string, 0), "", time.Unix(0, 0), time.Unix(0, 0), time.Unix(0, 0), 0)
	// timestamps to be initialized later

	// keep track of open directories
	stack = make([]*DirectoryEntry, 0, 1)
	stack = append(stack, root)
	stacks = make([]string, 0)

	// files are sorted so that first all child resources occur, and then those on the current level
	for _, file := range files {
		Log.Debug("Have file " + file.Name + " at path " + strings.Join(file.Path, "/"))

		// check if currently open directory matches this file's path
		// by counting matching levels
		var commonprefix []string = LongestCommonPrefix(stacks, file.Path)
		Log.Debug("Have common prefix " + strings.Join(commonprefix, "/"))

		// pop from path stack till arriving at common prefix
		if level > len(commonprefix) {
			if len(commonprefix) == 0 {
				// no common prefix!
				stack = stack[0:1]         // includes root
				stacks = make([]string, 0) // no path components
			} else {
				stack = stack[0:level]       // root + matching components
				stacks = stacks[0 : level-1] // matching components
			}
			level = len(commonprefix)
			Log.Debug("reduced level to " + strconv.Itoa(level) + " at " + strings.Join(stacks, "/"))
		}

		// create missing levels of path
		for level < len(file.Path) {
			var dirpath []string
			var dirname string

			// extract path and filename for this missing level
			if level == 0 {
				dirpath = make([]string, 0) // parent is root
			} else {
				dirpath = file.Path[0:level]
			}
			dirname = file.Path[level]
			// create directory node

			var d *DirectoryEntry = DirectoryEntry_NewFolder(dirpath, dirname, time.Unix(0, 0), time.Unix(0, 0), time.Unix(0, 0), 0)
			// timestamps to be initialized later

			// add to parent
			stack[level].Files = append(stack[level].Files, d)

			// jump to next level
			stack = append(stack, d)
			stacks = append(stacks, d.Name)
			level++
			Log.Debug("increased level to " + strconv.Itoa(level) + " at " + strings.Join(stacks, "/"))
		}

		// now put file as leaf
		stack[level].Files = append(stack[level].Files, file)
	}

	return root
}

// Print prints all elements of the directory tree by name and path
func (e *DirectoryEntry) Print() {
	fmt.Println("Looking at file " + e.Name + " at " + strings.Join(e.Path, "/"))

	if e.Type == DEFolder {
		for _, leaf := range e.Files {
			leaf.Print()
		}
	}
}

const int64Max int64 = 9223372036854775807

func fixTreeWithTimestampsFromLeafs(root *DirectoryEntry) {
	// nothing to do on leafs
	if root.Type == DEFile {
		return
	}

	var atimeMax time.Time = time.Unix(0, 0)
	var ctimeMin time.Time = time.Unix(int64Max, int64Max)
	var mtimeMax time.Time = time.Unix(0, 0)

	// from all children find:
	// maximum atime
	// minimum ctime
	// maximum mtime
	for _, e := range root.Files {
		// first fix child
		fixTreeWithTimestampsFromLeafs(e)

		// then choose timestamps
		if diff := e.ATime.Sub(atimeMax); diff > 0 {
			atimeMax = e.ATime
		}
		if diff := e.CTime.Sub(ctimeMin); diff < 0 {
			ctimeMin = e.CTime
		}
		if diff := e.MTime.Sub(mtimeMax); diff > 0 {
			mtimeMax = e.MTime
		}
	}

	// update self
	root.ATime = atimeMax
	root.CTime = ctimeMin
	root.MTime = mtimeMax
}

// mark meta.html and add it's content to the parent directory
func fixTreeWithMetafile(root *DirectoryEntry, client *minio.Client) {
	// find meta.html, if any
	for _, e := range root.Files {
		if e.Type == DEFolder {
			// recurse into subfolders
			fixTreeWithMetafile(e, client)
		} else if e.Name == MetaFilename {
			// found meta file
			e.IsMeta = true
			// fetch it's content
			object, err := client.GetObject(BucketName, fullPath(e), minio.GetObjectOptions{})
			if err != nil {
				Log.Error("failed to fetch meta file " + fullPath(e) + ": " + err.Error())
				continue
			}
			buffer := bytes.NewBuffer(nil)
			if _, err := io.Copy(buffer, object); err != nil {
				Log.Error("failed to copy meta file to buffer: " + err.Error())
				continue
			}

			// add to directory entry
			root.Meta = template.HTML(buffer.Bytes())
		}
	}
}

// IndexBucket creates the directory index for the bucket
func IndexBucket() *DirectoryEntry {
	var err error
	var client *minio.Client
	var done chan struct{} = make(chan struct{})
	var object minio.ObjectInfo
	var files []*DirectoryEntry
	var tree *DirectoryEntry

	// connect to S3
	client, err = minio.New(Endpoint, AccessKeyID, SecretAccessKey, UseSSL)
	if err != nil {
		Log.Error("couldn't initialize minio client")
		return nil
	}

	// get all objects
	files = make([]*DirectoryEntry, 0)
	defer close(done)
	for object = range client.ListObjectsV2WithMetadata(BucketName, "", true, done) {
		files = append(files, DirectoryEntry_FromObject(object))
	}

	// TODO: check if 'recursive' option to ListObjects already achieves strict tree order
	// TODO: sort if necessary

	tree = BuildDirectoryTree(files)

	// finally fill in folder timestamps
	fixTreeWithTimestampsFromLeafs(tree)

	// add meta.html content to folders
	fixTreeWithMetafile(tree, client)

	return tree
}

func fileSize(f *DirectoryEntry) string {
	return byteToMB(f.Size)
}

func fileType(f *DirectoryEntry) string {
	// folders are easy
	if f.Type == DEFolder {
		return "folder"
	}

	// otherwise use suffix after last dot
	parts := strings.Split(f.Name, ".")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}

	// fall back to nothing
	return ""
}

// return full path including name
func fullPath(f *DirectoryEntry) string {
	var path string
	// TODO: make efficient

	if len(f.Path) == 0 {
		// only filename component
		path = f.Name
	} else {
		// join path and filename with '/'
		path = strings.Join(f.Path, "/") + "/" + f.Name
	}

	// as a trick, let directories end with a trailing /
	if f.Type == DEFolder {
		path += "/"
	}

	// print files with full path and filename appended
	return path
}

// RenderHTML reads the index template and provides the result bytes
func (e *DirectoryEntry) RenderHTML() ([]byte, error) {
	funcMap := make(map[string]interface{})
	funcMap["fileSize"] = fileSize
	funcMap["fileType"] = fileType
	funcMap["fullPath"] = fullPath
	var indexTpl bytes.Buffer
	ft, err := template.New("index.html").Funcs(funcMap).ParseFiles("./templates/index.html")
	if err != nil {
		return nil, err
	}
	err = ft.Execute(&indexTpl, e)
	if err != nil {
		return nil, err
	}
	return indexTpl.Bytes(), nil
}

// CreateMinioClient returns a new client
func CreateMinioClient() (client *minio.Client, err error) {
	client, err = minio.New(Endpoint, AccessKeyID, SecretAccessKey, UseSSL)
	if err != nil {
		return
	}
	return
}

// 2020-06-15 copied from https://yourbasic.org/golang/formatting-byte-size-to-human-readable-format/
func byteToMB(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}

// LongestCommonPrefix finds the longest common prefix of 2 string slices
func LongestCommonPrefix(a, b []string) []string {
	var maxlength int
	var matches int

	// get length of smallest slice
	if len(a) < len(b) {
		maxlength = len(a)
	} else {
		maxlength = len(b)
	}

	// iterate over all elements and count matches
	for i := 0; i < maxlength; i++ {
		if a[i] == b[i] {
			matches++
		} else {
			// abort on first mismatch
			break
		}
	}

	if matches == 0 {
		return make([]string, 0)
	} else {
		return a[0:matches]
	}
}
