package bodydecoder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// ProtoRegistry holds compiled proto file descriptors and provides
// service/method lookup for named message decoding.
type ProtoRegistry struct {
	// methods maps "package.Service/Method" → MethodDescriptor.
	methods map[string]protoreflect.MethodDescriptor
}

func emptyRegistry() *ProtoRegistry {
	return &ProtoRegistry{methods: map[string]protoreflect.MethodDescriptor{}}
}

// LoadProtoFiles compiles .proto files from the given paths into a registry.
// Paths may be files or directories (recursively globbed for *.proto).
// Extra includeDirs are added to the import search path (like protoc -I).
// Returns the registry and any non-fatal errors (bad files are skipped).
func LoadProtoFiles(paths []string, includeDirs ...string) (*ProtoRegistry, []error) {
	protoFiles, importDirs, errs := resolveProtoPaths(paths)
	if len(protoFiles) == 0 {
		return emptyRegistry(), errs
	}

	importDirs = append(importDirs, includeDirs...)

	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: importDirs,
		}),
	}

	compiled, err := compiler.Compile(context.Background(), protoFiles...)
	if err != nil {
		errs = append(errs, fmt.Errorf("proto compile: %w", err))
		return emptyRegistry(), errs
	}

	reg := emptyRegistry()
	for _, f := range compiled {
		services := f.Services()
		for i := range services.Len() {
			svc := services.Get(i)
			methods := svc.Methods()
			for j := range methods.Len() {
				m := methods.Get(j)
				key := string(svc.FullName()) + "/" + string(m.Name())
				reg.methods[key] = m
			}
		}
	}
	return reg, errs
}

// LookupMethod finds a method descriptor by matching the gRPC path suffix.
// The path format is /{prefix}/{package.Service}/{Method}; the last two
// segments are used for lookup.
func (r *ProtoRegistry) LookupMethod(requestPath string) (protoreflect.MethodDescriptor, bool) {
	if r == nil || len(r.methods) == 0 {
		return nil, false
	}
	svc, method := extractGRPCPath(requestPath)
	if svc == "" || method == "" {
		return nil, false
	}
	m, ok := r.methods[svc+"/"+method]
	return m, ok
}

// DecodeNamed attempts to decode body as a named protobuf message.
// isRequest selects input vs output message type.
func (r *ProtoRegistry) DecodeNamed(body []byte, requestPath string, isRequest bool) (string, error) {
	msg, err := r.newMessage(requestPath, isRequest)
	if err != nil {
		return "", err
	}

	if err := proto.Unmarshal(body, msg); err != nil {
		return "", fmt.Errorf("unmarshal %s: %w", msg.Descriptor().FullName(), err)
	}

	opts := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		EmitUnpopulated: false,
	}
	out, err := opts.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("marshal json: %w", err)
	}
	return string(out), nil
}

// EncodeNamed encodes a JSON body back into protobuf wire format using the
// named method descriptor. isRequest selects input vs output message type.
func (r *ProtoRegistry) EncodeNamed(jsonBody []byte, requestPath string, isRequest bool) ([]byte, error) {
	msg, err := r.newMessage(requestPath, isRequest)
	if err != nil {
		return nil, err
	}

	if err := protojson.Unmarshal(jsonBody, msg); err != nil {
		return nil, fmt.Errorf("unmarshal json into %s: %w", msg.Descriptor().FullName(), err)
	}

	wire, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal proto: %w", err)
	}
	return wire, nil
}

// newMessage resolves the method descriptor for requestPath and returns a new
// dynamic message for either the request (input) or response (output) type.
func (r *ProtoRegistry) newMessage(requestPath string, isRequest bool) (*dynamicpb.Message, error) {
	method, ok := r.LookupMethod(requestPath)
	if !ok {
		return nil, fmt.Errorf("no method descriptor for path %q", requestPath)
	}

	var msgDesc protoreflect.MessageDescriptor
	if isRequest {
		msgDesc = method.Input()
	} else {
		msgDesc = method.Output()
	}
	return dynamicpb.NewMessage(msgDesc), nil
}

// HasMethods returns true if any service methods were loaded.
func (r *ProtoRegistry) HasMethods() bool {
	return r != nil && len(r.methods) > 0
}

// extractGRPCPath extracts "package.Service" and "Method" from a gRPC path.
// Handles paths with arbitrary prefix: /prefix/package.Service/Method
func extractGRPCPath(path string) (service, method string) {
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return "", ""
	}
	// Last two segments are service and method.
	method = parts[len(parts)-1]
	service = parts[len(parts)-2]
	if service == "" || method == "" {
		return "", ""
	}
	return service, method
}

// resolveProtoPaths expands paths into individual .proto files and
// collects import root directories. For directory paths that contain a
// buf.lock, cached BSR module paths are added to the import dirs.
func resolveProtoPaths(paths []string) (files []string, importDirs []string, errs []error) {
	seen := map[string]bool{}
	dirSet := map[string]bool{}

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			errs = append(errs, fmt.Errorf("proto path %q: %w", p, err))
			continue
		}

		if info.IsDir() {
			dirSet[p] = true
			// Auto-resolve buf BSR dependencies from buf.lock.
			if bufDirs, err := resolveBufCache(p); err != nil {
				errs = append(errs, err)
			} else {
				for _, d := range bufDirs {
					dirSet[d] = true
				}
			}

			err := filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return nil // skip errors
				}
				if !d.IsDir() && strings.HasSuffix(path, ".proto") {
					rel, relErr := filepath.Rel(p, path)
					if relErr != nil {
						rel = path
					}
					if !seen[rel] {
						seen[rel] = true
						files = append(files, rel)
					}
				}
				return nil
			})
			if err != nil {
				errs = append(errs, fmt.Errorf("walk %q: %w", p, err))
			}
		} else {
			dir := filepath.Dir(p)
			dirSet[dir] = true
			base := filepath.Base(p)
			if !seen[base] {
				seen[base] = true
				files = append(files, base)
			}
		}
	}

	for d := range dirSet {
		importDirs = append(importDirs, d)
	}
	return files, importDirs, errs
}
