package models

import (
	"bytes"
	"fmt"
	. "github.com/maltegrosse/go-minio-list/log"
	"github.com/minio/minio-go/v6"
	"html/template"
	"io"
	"strconv"
	"strings"
	"time"
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

type DirList struct {
	Files      []*File
	Folders    []*Folder
	Path       string
	ParentPath string
	Meta       template.HTML
	Redirect   bool
	IsFile     bool
}

type File struct {
	ContentType  string
	Name         string
	Size         int64
	LastModified time.Time
	Path         string
	FileType     string
}
type Folder struct {
	Name         string
	LastModified time.Time
	Path         string
}

// ScanPath scanns the current path which is selected
func (dl *DirList) ScanPath(path string) error {
	// not a path?
	if !strings.HasSuffix(path, "/") {
		// is it a file?
		isFile, err := dl.isFile(path)
		if err != nil {
			return err
		}
		if isFile {
			dl.IsFile = true
		} else {
			// last chance, perhaps someone forgot a /
			dl.Redirect = true
			err := dl.scan(path + "/")
			if err != nil {
				return err
			}
		}
	} else {
		err := dl.scan(path)
		if err != nil {
			return err
		}
	}
	return nil
}
func (dl *DirList) scan(path string) error {
	sPath := strings.Split(path, "/")
	// not sure if I need the parent path
	if len(sPath) > 1 {
		dl.ParentPath = strings.Join(sPath[:len(sPath)-1], "/")
	}
	client, err := CreateMinioClient()
	if err != nil {
		return err
	}
	doneCh := make(chan struct{})
	defer close(doneCh)
	objectCh := client.ListObjectsV2WithMetadata(BucketName, path, true, doneCh)
	for object := range objectCh {
		if object.Err != nil {
			Log.Error(object.Err.Error())
			return object.Err
		}
		// remove the path from the obj.key (full path filename)
		cutFileName := strings.Replace(object.Key, path, "", 1)
		cutFileNameSlice := strings.Split(cutFileName, "/")
		// avoid null pointer
		if len(cutFileNameSlice) > 0 {
			//handle folders
			if len(cutFileNameSlice) > 1 {
				var folder Folder
				folder.Name = cutFileNameSlice[0]
				folder.Path = path + folder.Name + "/"
				folder.LastModified = object.LastModified
				// avoid having folders with >1 files in sub folders included more often
				if !dl.folderAlreadyScanned(folder.Path, object.LastModified) {
					dl.Folders = append(dl.Folders, &folder)
				}
			} else {
				// handle meta data file
				if cutFileNameSlice[0] == MetaFilename {
					obj, err := client.GetObject(BucketName, object.Key, minio.GetObjectOptions{})
					if err != nil {
						Log.Error(err.Error())
						return err
					}
					buf := bytes.NewBuffer(nil)
					if _, err := io.Copy(buf, obj); err != nil {
						Log.Error(err.Error())
						return err
					}
					dl.Meta = template.HTML(buf.Bytes())
				} else {
					// handle files
					var file File
					file.Name = cutFileNameSlice[0]
					file.Size = object.Size
					file.LastModified = object.LastModified
					for metadata_key, metadata_value := range object.UserMetadata {
						if(metadata_key == "X-Amz-Meta-Mc-Attrs") {
							for _, attribute := range strings.Split(metadata_value, "/") {
								var kv = strings.Split(attribute, ":")
								if(len(kv) == 2) {
									if(kv[0] == "mtime") {
										v, err := strconv.ParseInt(kv[1], 10, 64)
										if(err == nil) {
											file.LastModified = time.Unix(v, 0)
										}
									}
								}
							}
						}
					}
					tmpFileTypes := strings.Split(file.Name, ".")
					file.FileType = tmpFileTypes[len(tmpFileTypes)-1]
					file.Path = path + file.Name
					dl.Files = append(dl.Files, &file)
				}

			}
		}

	}
	return nil
}

func (dl *DirList) folderAlreadyScanned(path string, lastModified time.Time) bool {
	for _, f := range dl.Folders {
		if f.Path == path {
			if f.LastModified.Unix() < lastModified.Unix() {
				// use always the latest date as we scan the files anyway
				f.LastModified = lastModified
			}
			return true
		}
	}
	return false
}
func (dl DirList) isFile(path string) (bool, error) {
	client, err := CreateMinioClient()
	if err != nil {
		return false, err
	}
	_, err = client.StatObject(BucketName, path, minio.StatObjectOptions{})
	if err != nil {
		Log.Error(err.Error())
		return false, nil
	}
	// no error means its a file
	return true, nil
}

// RenderHtml reads the index template and provides the result bytes
func (dl *DirList) RenderHtml() ([]byte, error) {
	funcMap := make(map[string]interface{})
	funcMap["byteToMB"] = dl.byteToMB
	var indexTpl bytes.Buffer
	ft, err := template.New("index.html").Funcs(funcMap).ParseFiles("./templates/index.html")
	if err != nil {
		return nil, err
	}
	err = ft.Execute(&indexTpl, dl)
	if err != nil {
		return nil, err
	}
	return indexTpl.Bytes(), nil
}

// 2020-06-15 copied from https://yourbasic.org/golang/formatting-byte-size-to-human-readable-format/
func (dl DirList) byteToMB(b int64) string {
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

// CreateMinioClient returns a new client
func CreateMinioClient() (client *minio.Client, err error) {
	client, err = minio.New(Endpoint, AccessKeyID, SecretAccessKey, UseSSL)
	if err != nil {
		return
	}
	return
}
