package apis

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/god-jason/iot-master/pkg/api"
)

var UploadPath = "." //TODO 添加到参数

func init() {
	api.RegisterUnAuthorized("POST", "upload", fileUpload)
	api.RegisterUnAuthorized("GET", "download/*filepath", fileDownload)
}

func filename(raw string, num int) string {
	now := time.Now()
	dir := fmt.Sprintf("%d/%d", now.Year(), now.Month())
	_ = os.MkdirAll(dir, os.ModePerm)
	return fmt.Sprintf("%d/%d/%d-%d", now.Year(), now.Month(), now.UnixMilli(), num) + filepath.Ext(raw)
}

func handleUpload(ff *multipart.FileHeader, num int) (string, error) {
	file, err := ff.Open()
	if err != nil {
		return "", err
	}

	now := time.Now()
	dir := fmt.Sprintf("%d/%d", now.Year(), now.Month())
	_ = os.MkdirAll(UploadPath+"/"+dir, os.ModePerm)
	// 保留经安全清洗的原始文件名(如 fotademo_1122.001.001_LuatOS-SoC_EC618.bin)，
	// 使下载URL可读、且固件版本号可从文件名解析
	fn := fmt.Sprintf("%d/%d/%d-%d-%s", now.Year(), now.Month(), now.UnixMilli(), num, sanitizeFilename(filepath.Base(ff.Filename)))

	file2, err := os.Create(UploadPath + "/" + fn)
	if err != nil {
		return "", err
	}
	defer file2.Close()

	_, err = io.Copy(file2, file)
	return fn, err
}

// sanitizeFilename 只保留中文、字母、数字和 ._- ，其余字符替换为下划线，防止路径异常
func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		case r >= 0x4e00 && r <= 0x9fa5:
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "file"
	}
	return b.String()
}

func fileUpload(ctx *gin.Context) {
	form, err := ctx.MultipartForm()
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	var files []string

	scheme := "http"
	if ctx.Request.TLS != nil {
		scheme = "https"
	}
	url := scheme + "://" + ctx.Request.Host + "/api/download/"

	i := 1
	for _, f := range form.File {
		for _, ff := range f {
			i++
			fn, err := handleUpload(ff, i)
			if err != nil {
				_ = ctx.Error(err)
				return
			}

			files = append(files, url+fn)
		}
	}
	api.OK(ctx, files)
}

func fileDownload(ctx *gin.Context) {
	fn := ctx.Param("filepath")
	ctx.File(UploadPath + fn)
}
