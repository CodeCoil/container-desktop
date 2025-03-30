package rewrite

import (
	"testing"
	"go.uber.org/zap/zaptest"
)

func runTest(t *testing.T, rewriteType RewriteType, wslDistroName string, mount string, expected string) string {
	logger := zaptest.NewLogger(t).Sugar()
	path := "/mnt/host"
	if wslDistroName != "" {
		path = "/mnt/wsl/" + wslDistroName
	}
	ctx := rewriteContext{ logger: logger, path: path, rewriteType: rewriteType, isWindows: wslDistroName == "" }
	
	mapping := mount+ ":/test"
	
	res := mapPath(mapping, &ctx)
	expected +=":/test"
	if res != expected  {
		logger.Errorf("FAILED. %s != %s", res, expected)
	}
	return ""
}

func TestLinuxRequest(t *testing.T){

	runTest(t, Request, "Ubuntu", "/", "/mnt/wsl/Ubuntu/")
	runTest(t, Request, "Ubuntu", "/mnt/c", "/mnt/host/c")
	runTest(t, Request, "Ubuntu", "/home/user", "/mnt/wsl/Ubuntu/home/user")
	runTest(t, Request, "Alpine", "/mnt/wsl/Ubuntu/home/user", "/mnt/wsl/Ubuntu/home/user")
	

	runTest(t, Response, "Ubuntu", "/mnt/host/c", "/mnt/c")
	// runTest(t, Response, "Ubuntu", "/mnt/host", "/mnt")
	runTest(t, Response, "Ubuntu", "/mnt/wsl/Ubuntu/home/user", "/home/user")
	runTest(t, Response, "Alpine", "/mnt/wsl/Ubuntu/home/user", "/mnt/wsl/Ubuntu/home/user")
	

}

func TestWindowsRequest(t *testing.T){

	runTest(t, Request, "", "C:\\users\\user", "/mnt/host/c/users/user")
	runTest(t, Request, "", "C:\\Users\\User", "/mnt/host/c/Users/User")
	runTest(t, Request, "", "\\\\wsl.localhost\\Ubuntu", "/mnt/wsl/Ubuntu") 
	runTest(t, Request, "", "\\\\wsl.localhost\\Ubuntu\\", "/mnt/wsl/Ubuntu/") 
	runTest(t, Request, "", "\\\\wsl.localhost\\Ubuntu\\user", "/mnt/wsl/Ubuntu/user") 
	runTest(t, Request, "", "\\\\wsl.localhost\\Ubuntu\\user\\", "/mnt/wsl/Ubuntu/user/") 
	runTest(t, Request, "", "\\\\wsl.localhost\\Ubuntu\\user\\home", "/mnt/wsl/Ubuntu/user/home") 
	runTest(t, Request, "", "\\\\wsl.localhost\\Ubuntu\\User\\Home", "/mnt/wsl/Ubuntu/User/Home") 
	runTest(t, Request, "", "\\\\wsl$\\Ubuntu\\user\\home", "/mnt/wsl/Ubuntu/user/home") 
	runTest(t, Request, "", "\\\\wsl$\\Ubuntu\\User\\Home", "/mnt/wsl/Ubuntu/User/Home") 
	runTest(t, Request, "", "/mnt/c", "/mnt/host/c") 
	runTest(t, Request, "", "/mnt/wsl/Ubuntu/user/home", "/mnt/wsl/Ubuntu/user/home")
	

	runTest(t, Response, "", "/mnt/host/c", "c:\\") 
	runTest(t, Response, "", "/mnt/host", "c:\\") 
	runTest(t, Response, "", "/mnt/host/c/users/user", "c:\\users\\user") 
	runTest(t, Response, "", "/mnt/wsl/Ubuntu", "\\\\wsl.localhost\\Ubuntu") 
	runTest(t, Response, "", "/mnt/wsl/Ubuntu/", "\\\\wsl.localhost\\Ubuntu\\") 
	runTest(t, Response, "", "/mnt/wsl/Ubuntu/user", "\\\\wsl.localhost\\Ubuntu\\user") 
	runTest(t, Response, "", "/mnt/wsl/Ubuntu/user/", "\\\\wsl.localhost\\Ubuntu\\user\\") 
	runTest(t, Response, "", "/mnt/wsl/Ubuntu/user/home", "\\\\wsl.localhost\\Ubuntu\\user\\home") 
}

func TestSocketMapping(t *testing.T){
	
	runTest(t, Request, "Ubuntu", "//var/run/docker.sock", "//var/run/docker.sock")
	runTest(t, Response, "Ubuntu", "//var/run/docker.sock", "//var/run/docker.sock")

	// Docker compatible
	runTest(t, Request, "", "//var/run/docker.sock", "//var/run/docker.sock")
	runTest(t, Response, "", "//var/run/docker.sock", "//var/run/docker.sock")
}