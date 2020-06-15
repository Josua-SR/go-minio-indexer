# Go-Minio/S3 Bucket Indexer
![GO](gopher-mi.jpg)

[![License](http://img.shields.io/:license-mit-blue.svg?style=flat-square)](http://badges.mit-license.org)
### General
Simple browse folders and download files from a minio/s3 bucket in _old school directory listing_ style.
### Requirements
- Go Lang > 1.13
- Minio/S3 bucket 
- Docker

### Config

Following parameters are required:
- service hostname and port
- minio/s3 credentials

If an additional note should be added to the index file - a meta.html can be placed in every folder.

This application can act as a reverse proxy or just forward to the target file.

### Build
```
docker build -t go-minio-indexer:latest --build-arg ARCH=amd64 .
```

### Run
```
docker run -p 5555:5555 -ti go-minio-indexer
```
### Usage
just open your browser and point to your localhost and predefined port.


### Todos
- Include static css files

## License
Copyright 2020 by Malte Grosse under
[MIT](https://choosealicense.com/licenses/mit/)

- File-Icons by [Daniel M. Hendricks](https://fileicons.org/) 
- Gopher by https://github.com/egonelbre/gophers
- Minio Logo by https://min.io/logo
