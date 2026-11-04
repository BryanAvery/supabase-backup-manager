package websession

import (
	"bytes"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Frontend assets are loaded *once* into memory from an explicitly configured
// build directory. No request ever traverses or reads the filesystem.
type staticAssets struct {
	index []byte
	files map[string][]byte
}
const maxAssetBytes = 4 << 20
const maxTotalAssetBytes = 8 << 20
var assetNamePattern=regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.(?:js|css)$`)

func safeRegular(path string, limit int64) ([]byte,error) {
	info,err:=os.Lstat(path)
	if err!=nil{return nil,fmt.Errorf("inspect web asset: %w",err)}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0022!=0 || info.Size()<1 || info.Size()>limit {
		return nil,errors.New("web asset must be a bounded regular file, not writable by group/other")
	}
	return os.ReadFile(path)
}
func safeDirectory(path string)error {
	info,err:=os.Lstat(path)
	if err!=nil{return fmt.Errorf("inspect web asset directory: %w",err)}
	if !info.IsDir() || info.Mode().Perm()&0022!=0 {
		return errors.New("web asset directory must be a private unmodified directory, not group/other writable or a symlink")
	}
	return nil
}

// loadAssets refuses symlinks, oversized contents, unrecognised extensions,
// malformed build output and directories writable by untrusted users. The
// operator must supply the Vite production dist directory, not source files.
func loadAssets(directory string)(*staticAssets,error) {
	if directory=="" || !filepath.IsAbs(directory) {
		return nil,errors.New("web assets require an absolute production build directory")
	}
	if err:=safeDirectory(directory);err!=nil{return nil,err}
	index,err:=safeRegular(filepath.Join(directory,"index.html"),64<<10)
	if err!=nil{return nil,err}
	if !bytes.Contains(index,[]byte(`id="root"`)) ||
		!bytes.Contains(index,[]byte("/app/assets/")) ||
		bytes.Contains(index,[]byte("http://")) {
		return nil,errors.New("web index does not look like the same-origin production application")
	}
	assetDir:=filepath.Join(directory,"assets")
	if err:=safeDirectory(assetDir);err!=nil{return nil,err}
	entries,err:=os.ReadDir(assetDir)
	if err!=nil{return nil,err}
	if len(entries)==0 || len(entries)>64{return nil,errors.New("invalid web asset count")}
	total:=len(index)
	assets:=&staticAssets{index:index,files:make(map[string][]byte)}
	for _,entry:=range entries{
		if !assetNamePattern.MatchString(entry.Name()){
			return nil,errors.New("unrecognised or potentially unsafe web asset name")
		}
		if entry.Type()&os.ModeSymlink!=0 || !entry.Type().IsRegular(){
			return nil,errors.New("web assets cannot contain symlinks or directories")
		}
		data,err:=safeRegular(filepath.Join(assetDir,entry.Name()),maxAssetBytes)
		if err!=nil{return nil,err}
		total+=len(data)
		if total>maxTotalAssetBytes{return nil,errors.New("web asset bundle exceeds size limit")}
		assets.files[entry.Name()]=data
	}
	return assets,nil
}

func(h *Handler)serveAssets(w http.ResponseWriter,r *http.Request){
	if r.Method!=http.MethodGet && r.Method!=http.MethodHead{
		w.Header().Set("Allow","GET, HEAD")
		jsonError(w,http.StatusMethodNotAllowed)
		return
	}
	if r.URL.RawQuery!="" {
		jsonError(w,http.StatusBadRequest)
		return
	}
	if r.URL.Path=="/app" {
		http.Redirect(w,r,"/app/",http.StatusPermanentRedirect)
		return
	}
	if r.URL.Path=="/app/" {
		w.Header().Set("Content-Type","text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; "+
			"img-src 'self' data:; font-src 'self'; base-uri 'none'; form-action 'none'; "+
			"frame-ancestors 'none'; object-src 'none'")
		w.Header().Set("X-Frame-Options","DENY")
		w.Header().Set("Referrer-Policy","no-referrer")
		w.Header().Set("Content-Length",fmt.Sprint(len(h.assets.index)))
		w.WriteHeader(http.StatusOK)
		if r.Method==http.MethodGet{_,_=w.Write(h.assets.index)}
		return
	}
	name:=strings.TrimPrefix(r.URL.Path,"/app/assets/")
	if !assetNamePattern.MatchString(name) || !strings.HasPrefix(r.URL.Path,"/app/assets/"){
		jsonError(w,http.StatusNotFound)
		return
	}
	data,ok:=h.assets.files[name]
	if !ok{jsonError(w,http.StatusNotFound);return}
	ext:=filepath.Ext(name)
	mimeType:=mime.TypeByExtension(ext)
	if ext==".js"{mimeType="text/javascript; charset=utf-8"}
	if ext==".css"{mimeType="text/css; charset=utf-8"}
	w.Header().Set("Content-Type",mimeType)
	w.Header().Set("Content-Length",fmt.Sprint(len(data)))
	w.WriteHeader(http.StatusOK)
	if r.Method==http.MethodGet{_,_=w.Write(data)}
}
