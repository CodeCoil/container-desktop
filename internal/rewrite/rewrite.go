package rewrite

import (
	"fmt"
	"encoding/json"
	"io"
	"regexp"
	rt "runtime"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type RewriteType int

// Indicate if the mapping need to translate between the 
// client file system and the docker daemon host file system 
// (rewriteType: Request) or vice versa (rewriteType: Response)
const (
	Request RewriteType = iota + 1 // EnumIndex = 1
	Response
)
type rewriteContext struct {
	// logger
	logger *zap.SugaredLogger
	// The base virtual mapping mount folder that
	// provide access either to the host or to the 
	// local WSL distro file system.
	// Can be either:
	// Windows: /mnt/distro 
	// Linux  : /mnt/wsl/{wsl_distro} (e.g. /mnt/wsl/{Ubuntu})
	path string
	// Indicate if the mapping need to translate between the 
	// client file system and the docker daemon host file system 
	// (rewriteType: Request) or vice versa (rewriteType: Response)
	rewriteType RewriteType
	// Indicate if the proxy is running on Windows (note that
	// `isWindow == true IMPLY THAT host == '/mnt/distro'`)
	isWindows bool
}

type rewriteMapItem struct {
	method   string
	pattern  string
	rewriter func(map[string]interface{}, *rewriteContext)
}

var rewriteMappings = []rewriteMapItem{
	{"GET", `(/.*?)?/containers(/.*?)?/json`, rewriteContainerSummary},
	{"POST", `(/.*?)?/containers/create`, rewriteContainerConfig},
	{"POST", `(/.*?)?/services/create`, rewriteServiceSpec},
	{"POST", `(/.*?)?/services/(/.*?)/update`, rewriteServiceSpec},
	{"GET", `(/.*?)?/services(/.*?)$`, rewriteService},
	{"GET", `(/.*?)?/tasks(/.*?)?`, rewriteTask},
}

func enabled(logger *zap.SugaredLogger, level zapcore.Level) bool {
	return logger.Desugar().Core().Enabled(level)
}

func RewriteBody(body io.ReadCloser, urlPath string, wslDistroName string, logger *zap.SugaredLogger, rewriteType RewriteType) (rewrittenBody []byte, err error) {
	if body != nil {
		rewriter, ok := getRewriter(urlPath)
		if ok {
			buf, err := io.ReadAll(body)
			if enabled(logger, zapcore.DebugLevel) {
				logger.Debugf("Original body: %s", string(buf))
			}
			if err != nil {
				return nil, err
			}
			if len(buf) == 0 {
				return buf, nil
			}
			var jsonArray []interface{}
			isArray := false
			if buf[0] == '{' {
				logger.Debug("Body is a JSON object")
				jsonMap := make(map[string]interface{})
				err = json.Unmarshal(buf, &jsonMap)
				if err != nil {
					return nil, err
				}
				jsonArray = make([]interface{}, 1)
				jsonArray[0] = jsonMap
			} else if buf[0] == '[' {
				logger.Debug("Body is a JSON array")
				isArray = true
				err := json.Unmarshal(buf, &jsonArray)
				if err != nil {
					return nil, err
				}
			}
			if jsonArray != nil {
				path := "/mnt/"
				if len(wslDistroName) > 0 {
					path += "wsl/" + wslDistroName
				} else {
					path += "host"
				}
				logger.Debugf("Rewrite with base path: %s", path)
				for _, item := range jsonArray {
					m, ok := item.(map[string]interface{})
					if ok {
						ctx := &rewriteContext{ logger: logger, path: path, rewriteType: rewriteType, isWindows: rt.GOOS == "windows" }
						rewriter(m, ctx)
					}
				}
				if isArray {
					buf, err = json.Marshal(jsonArray)
				} else {
					buf, err = json.Marshal(jsonArray[0])
				}
				if err != nil {
					return nil, err
				}

				if enabled(logger, zapcore.DebugLevel) {
					logger.Debugf("Rewritten body: %s", string(buf))
				}
				return buf, nil
			}
		}
	}
	return nil, nil
}

func getRewriter(urlPath string) (func(map[string]interface{}, *rewriteContext), bool) {
	for _, item := range rewriteMappings {
		ok, err := regexp.MatchString(item.pattern, urlPath)
		if err == nil && ok {
			return item.rewriter, true
		}
	}
	return nil, false
}

func rewriteContainerSummary(jsonMap map[string]interface{}, context *rewriteContext) {
	o, ok := jsonMap["HostConfig"]
	if ok {
		hostConfig, ok := o.(map[string]interface{})
		if ok {
			rewriteHostConfig(hostConfig, context)
		}
	}
	o, ok = jsonMap["Mounts"]
	if ok {
		mounts, ok := o.([]interface{})
		if ok {
			rewriteMounts(mounts, context)
		}
	}
}

func rewriteContainerConfig(jsonMap map[string]interface{}, context *rewriteContext) {
	o, ok := jsonMap["HostConfig"]
	if ok {
		hostConfig, ok := o.(map[string]interface{})
		if ok {
			rewriteHostConfig(hostConfig, context)
		}
	}
	o, ok = jsonMap["Mounts"]
	if ok {
		mounts, ok := o.([]interface{})
		if ok {
			rewriteMounts(mounts, context)
		}
	}
}

func rewriteService(jsonMap map[string]interface{}, context *rewriteContext) {
	o, ok := jsonMap["Spec"]
	if ok {
		spec, ok := o.(map[string]interface{})
		if ok {
			rewriteServiceSpec(spec, context)
		}
	}
}

func rewriteServiceSpec(jsonMap map[string]interface{}, context *rewriteContext) {
	o, ok := jsonMap["TaskTemplate"]
	if ok {
		taskSpec, ok := o.(map[string]interface{})
		if ok {
			rewriteTaskSpec(taskSpec, context)
		}
	}
}

func rewriteTaskSpec(jsonMap map[string]interface{}, context *rewriteContext) {
	o, ok := jsonMap["ContainerSpec"]
	if ok {
		containerSpec, ok := o.(map[string]interface{})
		if ok {
			rewriteContainerSpec(containerSpec, context)
		}
	}
}

func rewriteContainerSpec(jsonMap map[string]interface{}, context *rewriteContext) {
	o, ok := jsonMap["Mounts"]
	if ok {
		mounts, ok := o.([]interface{})
		if ok {
			rewriteMounts(mounts, context)
		}
	}
}

func rewriteTask(jsonMap map[string]interface{}, context *rewriteContext) {
	o, ok := jsonMap["Spec"]
	if ok {
		taskSpec, ok := o.(map[string]interface{})
		if ok {
			rewriteTaskSpec(taskSpec, context)
		}
	}
}

func rewriteHostConfig(hostConfig map[string]interface{}, context *rewriteContext) {
	o, ok := hostConfig["Binds"]
	if ok {
		binds, ok := o.([]interface{})
		if ok {
			for i, bind := range binds {
				s := bind.(string)
				s = mapPath(s, context)
				binds[i] = s
			}
		}
	}
	o, ok = hostConfig["Mounts"]
	if ok {
		mounts, ok := o.([]interface{})
		if ok {
			rewriteMounts(mounts, context)
		}
	}
}

func rewriteMounts(mounts []interface{}, context *rewriteContext) {
	for _, o := range mounts {
		mount, ok := o.(map[string]interface{})
		if ok {
			t := mount["Type"].(string)
			if t == "bind" {
				s := mount["Source"].(string)
				s = mapPath(s, context)
				mount["Source"] = s
			}
		}
	}
}

func mapPath(binding string, context *rewriteContext) string {

	if context != nil {
		return mapPathV2(binding, context)
	}

	rtype := "REQ"
	if context.rewriteType == Response {
		rtype = "RSP"
	}
	path := context.path
	logger := context.logger
	lctx := fmt.Sprintf("[mapPath2(type: %8s, win: %v, binding: %40s, base: %10s)]", rtype, context.isWindows, binding, path)
	
	s := strings.Replace(binding, "\\", "/", -1)
	parts := strings.Split(s, ":")
	if strings.HasPrefix(parts[0], "/mnt/host/") {
		p := parts[0][10:]
		parts2 := strings.Split(p, "/")
		p = parts2[0] + ":/" + strings.Join(parts2[1:], "/")
		parts[0] = strings.Replace(p, "/", "\\", -1)
		s = strings.Join(parts, ":")
	} else if strings.HasPrefix(parts[0], "/mnt/wsl/") {
		parts2 := strings.Split(parts[0][9:], "/")
		parts[0] = strings.Join(parts2[1:], "/")
		s = strings.Join(parts, ":")
	} else if rt.GOOS == "windows" {
		if parts[0] != "/" && len(parts[0]) == 1 {
			s = path + "/" + strings.ToLower(parts[0])
			s += strings.Join(parts[1:], ":")
		}
	} else if strings.HasPrefix(s, "/") {
		s = path + s
	}
	logger.Infof("%s ==> %-40s", lctx, s)
	return s
}

// Map a docker container volume bind expression between the WSL host
// context (e.g. '/mnt/host' on windows, or mnt/wsl/{distro_name} when
// the proxy is running in a WSL linux distribution) from which the 
// client is run and the host where the  docker daemon is running.
// s: container volume bind expression (e.g. {host_path}:{container_path})
// rewriteContext: provide contextual information about the client hosting 
// context.
func mapPathV2(binding string, context *rewriteContext) string {
	// "%s Request  %40s ==> [%10s] ==> %-40s", lctx, original, path, s
	rtype := "REQ"
	if context.rewriteType == Response {
		rtype = "RSP"
	}
	path := context.path
	logger := context.logger
	lctx := fmt.Sprintf("[mapPath2(type: %8s, win: %v, binding: %40s, base: %10s)]", rtype, context.isWindows, binding, path)

	// Before we normalize the slash in the path,
	// handle resources that are on the host 
	// and are not replicated through mounts
	// (e.g. //var/run/docker.sock)
	if strings.HasPrefix(binding, "//") {
		// Log the mapping result
		logger.Infof("%s ==> %-40s", lctx, binding)
		return binding
	}

	s := strings.Replace(binding, "\\", "/", -1)
	parts := strings.Split(s, ":")
	clientPath := parts[0]

	getUnixPath := func(base string, pathSegments ...string) string {
		x := append([]string{base}, pathSegments...)
		return strings.Join(x, "/")
	}
	getMountPath := func(pathSegments ...string) string {
		return getUnixPath("/mnt", pathSegments...)
	}
	getHostMountPath := func(pathSegments ...string) string {
		return getUnixPath("/mnt/host", pathSegments...)
	}
	getWslPath := func(pathSegments ...string) string {
		return getUnixPath("/mnt/wsl", pathSegments...)
	}
	translateToWindowsPath := func(linuxPath string) string {
		return strings.Replace(linuxPath, "/", "\\", -1)
	}

	if strings.HasPrefix(clientPath, "/mnt/") {
		// Handle paths starting with /mnt/
		mntPath := clientPath[5:]
		allPathSegments := strings.Split(mntPath, "/")
		mntType := allPathSegments[0]
		pathSegments := allPathSegments[1:]

		switch mntType {
		case "host":
			if len(pathSegments) == 0 {
				logger.Errorf("%s Invalid binding. Expected at least one path segment.", lctx)
				parts[0] = ""
				break;
			}
			// Handle host paths
			
			if context.isWindows {
				// Host proxy on Windows
				drive := pathSegments[0]
				drivePath := strings.Join(pathSegments[1:], "/")
				// winpath => {drive}:/some/path
				winpath := drive + ":/" + drivePath
				// parts[0] => {drive}:\some\path
				parts[0] = translateToWindowsPath(winpath)
			} else {
				// either a socket or a unix path
				// Distro proxy on WSL
				parts[0] = getMountPath(pathSegments...)
			}
		case "wsl":
			// Handle WSL paths
			distro := pathSegments[0]
			if context.isWindows && context.rewriteType == Response {
				// Host proxy on Windows
				res := getUnixPath("//wsl.localhost", pathSegments...)
				parts[0] = translateToWindowsPath(res)
			} else {
				// Distro proxy on WSL
				if context.rewriteType == Response && path == getWslPath(distro) {
					// When a request is coming from the local WSL distro
					// parts[0] => /some/path
					parts[0] = getUnixPath("", pathSegments[1:]...)
				} else {
					parts[0] = getWslPath(pathSegments...)
				}
			}
		default:
			if context.rewriteType == Request {
				parts[0] = getHostMountPath(allPathSegments...)
			} else {
				logger.Infof("%s Don't know how to map mount type %s (type: %s) for response ", lctx, parts[0], mntType)
			}
		}
		s = strings.Join(parts, ":")
	} else if context.isWindows {
		// Handle Windows paths

		// strip windows UNC prefix (case insensitive)
		wslPath := strings.ToLower(clientPath)
		if strings.HasPrefix(clientPath, "//wsl.localhost/") {
			wslPath = clientPath[16:]
		} else if strings.HasPrefix(clientPath, "//wsl$/") {
			wslPath = clientPath[7:]
		}
		// Check if we found either prefixes by comparing
		// with the length of the original value
		if len(wslPath) != len(clientPath)  {
			// Map as /wsl/{distro}/{wslPath}
			parts[0] = getWslPath(wslPath)
			s = strings.Join(parts, ":")
		} else if len(clientPath) == 1 {
			// Handle REQUESTS that contains host paths on 
			// the windows proxy.

			// This can only be a request because we always
			// map windows paths to /mnt/host/{path}
			// so for responses we should go through the
			// first leg.

			// ASSERT(context.rewriteType == Request)
			
			// At this point parts should be something like 
			// => [{drive == clientPath}, {path}, {containerPath}]
			drive := strings.ToLower(parts[0])

			// s => /mnt/host/{drive}/some/path:/{containerPath}
			s = path + "/" + drive + strings.Join(parts[1:], ":")
		} else {
			// Handle other Windows paths
			parts[0] = translateToWindowsPath(clientPath)
			s = strings.Join(parts, ":")
		}
	} else if strings.HasPrefix(s, "/") {
		// Handle Linux paths
		// This could result in either a mapping 
		// to the mount of the WSL distro OR
		// the mount of the host path
		s = path + s
	}

	// Log the mapping result
	logger.Infof("%s ==> %-40s", lctx, s)

	return s
}

