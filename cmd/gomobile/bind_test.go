// Copyright 2015 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"text/template"
)

func TestBindAndroid(t *testing.T) {
	androidHome := os.Getenv("ANDROID_HOME")
	if androidHome == "" {
		t.Skip("ANDROID_HOME not found, skipping bind")
	}
	platform, err := androidAPIPath()
	if err != nil {
		t.Skip("No android API platform found in $ANDROID_HOME, skipping bind")
	}
	platform = strings.Replace(platform, androidHome, "$ANDROID_HOME", -1)

	defer func() {
		xout = os.Stderr
		buildN = false
		buildX = false
		buildO = ""
		buildTarget = ""
		bindJavaPkg = ""
	}()
	buildN = true
	buildX = true
	buildO = "asset.aar"
	buildTarget = "android/arm"

	tests := []struct {
		javaPkg string
	}{
		{
			// Empty javaPkg
		},
		{
			javaPkg: "com.example.foo",
		},
	}
	for _, tc := range tests {
		bindJavaPkg = tc.javaPkg

		buf := new(bytes.Buffer)
		xout = buf
		gopath = filepath.SplitList(goEnv("GOPATH"))[0]
		if goos == "windows" {
			os.Setenv("HOMEDRIVE", "C:")
		}
		cmdBind.flag.Parse([]string{"golang.org/x/mobile/asset"})
		err := runBind(cmdBind)
		if err != nil {
			t.Log(buf.String())
			t.Fatal(err)
		}
		got := filepath.ToSlash(buf.String())

		output, err := defaultOutputData()
		if err != nil {
			t.Fatal(err)
		}
		data := struct {
			outputData
			AndroidPlatform string
			JavaPkg         string
		}{
			outputData:      output,
			AndroidPlatform: platform,
			JavaPkg:         tc.javaPkg,
		}

		wantBuf := new(bytes.Buffer)
		if err := bindAndroidTmpl.Execute(wantBuf, data); err != nil {
			t.Errorf("%+v: computing diff failed: %v", tc, err)
			continue
		}

		diff, err := diff(got, wantBuf.String())
		if err != nil {
			t.Errorf("%+v: computing diff failed: %v", tc, err)
			continue
		}
		if diff != "" {
			t.Errorf("%+v: unexpected output:\n%s", tc, diff)
		}
	}
}

func TestBindIOS(t *testing.T) {
	if !xcodeAvailable() {
		t.Skip("Xcode is missing")
	}
	defer func() {
		xout = os.Stderr
		buildN = false
		buildX = false
		buildO = ""
		buildTarget = ""
		bindPrefix = ""
	}()
	buildN = true
	buildX = true
	buildO = "Asset.xcframework"
	buildTarget = "ios/arm64"

	tests := []struct {
		prefix string
		out    string
	}{
		{
			// empty prefix
		},
		{
			prefix: "Foo",
		},
		{
			out: "Abcde.xcframework",
		},
	}
	for _, tc := range tests {
		bindPrefix = tc.prefix
		if tc.out != "" {
			buildO = tc.out
		}

		buf := new(bytes.Buffer)
		xout = buf
		gopath = filepath.SplitList(goEnv("GOPATH"))[0]
		if goos == "windows" {
			os.Setenv("HOMEDRIVE", "C:")
		}
		cmdBind.flag.Parse([]string{"golang.org/x/mobile/asset"})
		if err := runBind(cmdBind); err != nil {
			t.Log(buf.String())
			t.Fatal(err)
		}
		got := filepath.ToSlash(buf.String())

		output, err := defaultOutputData()
		if err != nil {
			t.Fatal(err)
		}

		data := struct {
			outputData
			Output         string
			Prefix         string
			BitcodeEnabled bool
		}{
			outputData:     output,
			Output:         buildO[:len(buildO)-len(".xcframework")],
			Prefix:         tc.prefix,
			BitcodeEnabled: bitcodeEnabled,
		}

		wantBuf := new(bytes.Buffer)
		if err := bindIOSTmpl.Execute(wantBuf, data); err != nil {
			t.Errorf("%+v: computing diff failed: %v", tc, err)
			continue
		}

		diff, err := diff(got, wantBuf.String())
		if err != nil {
			t.Errorf("%+v: computing diff failed: %v", tc, err)
			continue
		}
		if diff != "" {
			t.Errorf("%+v: unexpected output:\n%s", tc, diff)
		}
	}
}

var bindAndroidTmpl = template.Must(template.New("output").Parse(`GOMOBILE={{.GOPATH}}/pkg/gomobile
WORK=$WORK
GOOS=android CGO_ENABLED=1 gobind -lang=go,java -outdir=$WORK{{if .JavaPkg}} -javapkg={{.JavaPkg}}{{end}} golang.org/x/mobile/asset
mkdir -p $WORK/src
PWD=$WORK/src GOOS=android GOARCH=arm CC=$NDK_PATH/toolchains/llvm/prebuilt/{{.NDKARCH}}/bin/armv7a-linux-androideabi16-clang CXX=$NDK_PATH/toolchains/llvm/prebuilt/{{.NDKARCH}}/bin/armv7a-linux-androideabi16-clang++ CGO_ENABLED=1 GOARM=7 GOPATH=$WORK:$GOPATH go mod tidy
PWD=$WORK/src GOOS=android GOARCH=arm CC=$NDK_PATH/toolchains/llvm/prebuilt/{{.NDKARCH}}/bin/armv7a-linux-androideabi16-clang CXX=$NDK_PATH/toolchains/llvm/prebuilt/{{.NDKARCH}}/bin/armv7a-linux-androideabi16-clang++ CGO_ENABLED=1 GOARM=7 GOPATH=$WORK:$GOPATH go build -x -buildmode=c-shared -o=$WORK/android/src/main/jniLibs/armeabi-v7a/libgojni.so ./gobind
PWD=$WORK/java javac -d $WORK/javac-output -source 1.7 -target 1.7 -bootclasspath {{.AndroidPlatform}}/android.jar *.java
jar c -C $WORK/javac-output .
`))

var bindIOSTmpl = template.Must(template.New("output").Parse(`GOMOBILE={{.GOPATH}}/pkg/gomobile
WORK=$WORK
GOOS=ios CGO_ENABLED=1 gobind -lang=go,objc -outdir=$WORK -tags=ios{{if .Prefix}} -prefix={{.Prefix}}{{end}} golang.org/x/mobile/asset
rm -r -f "{{.Output}}.xcframework"
mkdir -p $WORK/src
PWD=$WORK/src GOOS=ios GOARCH=arm64 CC=iphonesimulator-clang CXX=iphonesimulator-clang++ CGO_CFLAGS=-isysroot=iphonesimulator -mios-simulator-version-min=13.0 {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_CXXFLAGS=-isysroot=iphonesimulator -mios-simulator-version-min=13.0 {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_LDFLAGS=-isysroot=iphonesimulator -mios-simulator-version-min=13.0 {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_ENABLED=1 GOPATH=$WORK:$GOPATH go mod tidy
PWD=$WORK/src GOOS=ios GOARCH=arm64 CC=iphonesimulator-clang CXX=iphonesimulator-clang++ CGO_CFLAGS=-isysroot=iphonesimulator -mios-simulator-version-min=13.0 {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_CXXFLAGS=-isysroot=iphonesimulator -mios-simulator-version-min=13.0 {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_LDFLAGS=-isysroot=iphonesimulator -mios-simulator-version-min=13.0 {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_ENABLED=1 GOPATH=$WORK:$GOPATH go build -tags ios -x -buildmode=c-archive -o $WORK/{{.Output}}-arm64.a ./gobind
mkdir -p $WORK/simulator/{{.Output}}.framework/Versions/A/Headers
ln -s A $WORK/simulator/{{.Output}}.framework/Versions/Current
ln -s Versions/Current/Headers $WORK/simulator/{{.Output}}.framework/Headers
ln -s Versions/Current/{{.Output}} $WORK/simulator/{{.Output}}.framework/{{.Output}}
xcrun lipo -create -arch arm64 $WORK/{{.Output}}-arm64.a -o $WORK/simulator/{{.Output}}.framework/Versions/A/{{.Output}}
cp $WORK/src/gobind/{{.Prefix}}Asset.objc.h $WORK/simulator/{{.Output}}.framework/Versions/A/Headers/{{.Prefix}}Asset.objc.h
mkdir -p $WORK/simulator/{{.Output}}.framework/Versions/A/Headers
cp $WORK/src/gobind/Universe.objc.h $WORK/simulator/{{.Output}}.framework/Versions/A/Headers/Universe.objc.h
mkdir -p $WORK/simulator/{{.Output}}.framework/Versions/A/Headers
cp $WORK/src/gobind/ref.h $WORK/simulator/{{.Output}}.framework/Versions/A/Headers/ref.h
mkdir -p $WORK/simulator/{{.Output}}.framework/Versions/A/Headers
mkdir -p $WORK/simulator/{{.Output}}.framework/Versions/A/Headers
mkdir -p $WORK/simulator/{{.Output}}.framework/Versions/A/Resources
ln -s Versions/Current/Resources $WORK/simulator/{{.Output}}.framework/Resources
mkdir -p $WORK/simulator/{{.Output}}.framework/Resources
mkdir -p $WORK/simulator/{{.Output}}.framework/Versions/A/Modules
ln -s Versions/Current/Modules $WORK/simulator/{{.Output}}.framework/Modules
mkdir -p $WORK/src
PWD=$WORK/src GOOS=ios GOARCH=arm64 CC=iphoneos-clang CXX=iphoneos-clang++ CGO_CFLAGS=-isysroot=iphoneos -miphoneos-version-min=13.0 {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_CXXFLAGS=-isysroot=iphoneos -miphoneos-version-min=13.0 {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_LDFLAGS=-isysroot=iphoneos -miphoneos-version-min=13.0 {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_ENABLED=1 GOPATH=$WORK:$GOPATH go mod tidy
PWD=$WORK/src GOOS=ios GOARCH=arm64 CC=iphoneos-clang CXX=iphoneos-clang++ CGO_CFLAGS=-isysroot=iphoneos -miphoneos-version-min=13.0 {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_CXXFLAGS=-isysroot=iphoneos -miphoneos-version-min=13.0 {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_LDFLAGS=-isysroot=iphoneos -miphoneos-version-min=13.0 {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_ENABLED=1 GOPATH=$WORK:$GOPATH go build -tags ios -x -buildmode=c-archive -o $WORK/{{.Output}}-arm64.a ./gobind
mkdir -p $WORK/ios/{{.Output}}.framework/Versions/A/Headers
ln -s A $WORK/ios/{{.Output}}.framework/Versions/Current
ln -s Versions/Current/Headers $WORK/ios/{{.Output}}.framework/Headers
ln -s Versions/Current/{{.Output}} $WORK/ios/{{.Output}}.framework/{{.Output}}
xcrun lipo -create -arch arm64 $WORK/{{.Output}}-arm64.a -o $WORK/ios/{{.Output}}.framework/Versions/A/{{.Output}}
cp $WORK/src/gobind/{{.Prefix}}Asset.objc.h $WORK/ios/{{.Output}}.framework/Versions/A/Headers/{{.Prefix}}Asset.objc.h
mkdir -p $WORK/ios/{{.Output}}.framework/Versions/A/Headers
cp $WORK/src/gobind/Universe.objc.h $WORK/ios/{{.Output}}.framework/Versions/A/Headers/Universe.objc.h
mkdir -p $WORK/ios/{{.Output}}.framework/Versions/A/Headers
cp $WORK/src/gobind/ref.h $WORK/ios/{{.Output}}.framework/Versions/A/Headers/ref.h
mkdir -p $WORK/ios/{{.Output}}.framework/Versions/A/Headers
mkdir -p $WORK/ios/{{.Output}}.framework/Versions/A/Headers
mkdir -p $WORK/ios/{{.Output}}.framework/Versions/A/Resources
ln -s Versions/Current/Resources $WORK/ios/{{.Output}}.framework/Resources
mkdir -p $WORK/ios/{{.Output}}.framework/Resources
mkdir -p $WORK/ios/{{.Output}}.framework/Versions/A/Modules
ln -s Versions/Current/Modules $WORK/ios/{{.Output}}.framework/Modules
mkdir -p $WORK/src
PWD=$WORK/src GOOS=darwin GOARCH=arm64 CC=macosx-clang CXX=macosx-clang++ CGO_CFLAGS=-isysroot=macosx -target arm64-apple-ios13.0-macabi {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_CXXFLAGS=-isysroot=macosx -target arm64-apple-ios13.0-macabi {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_LDFLAGS=-isysroot=macosx -target arm64-apple-ios13.0-macabi {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_ENABLED=1 GOPATH=$WORK:$GOPATH go mod tidy
PWD=$WORK/src GOOS=darwin GOARCH=arm64 CC=macosx-clang CXX=macosx-clang++ CGO_CFLAGS=-isysroot=macosx -target arm64-apple-ios13.0-macabi {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_CXXFLAGS=-isysroot=macosx -target arm64-apple-ios13.0-macabi {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_LDFLAGS=-isysroot=macosx -target arm64-apple-ios13.0-macabi {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_ENABLED=1 GOPATH=$WORK:$GOPATH go build -tags ios -x -buildmode=c-archive -o $WORK/{{.Output}}-arm64.a ./gobind
mkdir -p $WORK/catalyst/{{.Output}}.framework/Versions/A/Headers
ln -s A $WORK/catalyst/{{.Output}}.framework/Versions/Current
ln -s Versions/Current/Headers $WORK/catalyst/{{.Output}}.framework/Headers
ln -s Versions/Current/{{.Output}} $WORK/catalyst/{{.Output}}.framework/{{.Output}}
xcrun lipo -create -arch arm64 $WORK/{{.Output}}-arm64.a -o $WORK/catalyst/{{.Output}}.framework/Versions/A/{{.Output}}
cp $WORK/src/gobind/{{.Prefix}}Asset.objc.h $WORK/catalyst/{{.Output}}.framework/Versions/A/Headers/{{.Prefix}}Asset.objc.h
mkdir -p $WORK/catalyst/{{.Output}}.framework/Versions/A/Headers
cp $WORK/src/gobind/Universe.objc.h $WORK/catalyst/{{.Output}}.framework/Versions/A/Headers/Universe.objc.h
mkdir -p $WORK/catalyst/{{.Output}}.framework/Versions/A/Headers
cp $WORK/src/gobind/ref.h $WORK/catalyst/{{.Output}}.framework/Versions/A/Headers/ref.h
mkdir -p $WORK/catalyst/{{.Output}}.framework/Versions/A/Headers
mkdir -p $WORK/catalyst/{{.Output}}.framework/Versions/A/Headers
mkdir -p $WORK/catalyst/{{.Output}}.framework/Versions/A/Resources
ln -s Versions/Current/Resources $WORK/catalyst/{{.Output}}.framework/Resources
mkdir -p $WORK/catalyst/{{.Output}}.framework/Resources
mkdir -p $WORK/catalyst/{{.Output}}.framework/Versions/A/Modules
ln -s Versions/Current/Modules $WORK/catalyst/{{.Output}}.framework/Modules
mkdir -p $WORK/src
PWD=$WORK/src GOOS=darwin GOARCH=arm64 CC=macosx-clang CXX=macosx-clang++ CGO_CFLAGS=-isysroot=macosx {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_CXXFLAGS=-isysroot=macosx {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_LDFLAGS=-isysroot=macosx {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_ENABLED=1 GOPATH=$WORK:$GOPATH go mod tidy
PWD=$WORK/src GOOS=darwin GOARCH=arm64 CC=macosx-clang CXX=macosx-clang++ CGO_CFLAGS=-isysroot=macosx {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_CXXFLAGS=-isysroot=macosx {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_LDFLAGS=-isysroot=macosx {{if .BitcodeEnabled}}-fembed-bitcode {{end}}-arch arm64 CGO_ENABLED=1 GOPATH=$WORK:$GOPATH go build -tags ios -x -buildmode=c-archive -o $WORK/{{.Output}}-arm64.a ./gobind
mkdir -p $WORK/macos/{{.Output}}.framework/Versions/A/Headers
ln -s A $WORK/macos/{{.Output}}.framework/Versions/Current
ln -s Versions/Current/Headers $WORK/macos/{{.Output}}.framework/Headers
ln -s Versions/Current/{{.Output}} $WORK/macos/{{.Output}}.framework/{{.Output}}
xcrun lipo -create -arch arm64 $WORK/{{.Output}}-arm64.a -o $WORK/macos/{{.Output}}.framework/Versions/A/{{.Output}}
cp $WORK/src/gobind/{{.Prefix}}Asset.objc.h $WORK/macos/{{.Output}}.framework/Versions/A/Headers/{{.Prefix}}Asset.objc.h
mkdir -p $WORK/macos/{{.Output}}.framework/Versions/A/Headers
cp $WORK/src/gobind/Universe.objc.h $WORK/macos/{{.Output}}.framework/Versions/A/Headers/Universe.objc.h
mkdir -p $WORK/macos/{{.Output}}.framework/Versions/A/Headers
cp $WORK/src/gobind/ref.h $WORK/macos/{{.Output}}.framework/Versions/A/Headers/ref.h
mkdir -p $WORK/macos/{{.Output}}.framework/Versions/A/Headers
mkdir -p $WORK/macos/{{.Output}}.framework/Versions/A/Headers
mkdir -p $WORK/macos/{{.Output}}.framework/Versions/A/Resources
ln -s Versions/Current/Resources $WORK/macos/{{.Output}}.framework/Resources
mkdir -p $WORK/macos/{{.Output}}.framework/Resources
mkdir -p $WORK/macos/{{.Output}}.framework/Versions/A/Modules
ln -s Versions/Current/Modules $WORK/macos/{{.Output}}.framework/Modules
xcodebuild -create-xcframework -framework $WORK/simulator/{{.Output}}.framework -framework $WORK/ios/{{.Output}}.framework -framework $WORK/catalyst/{{.Output}}.framework -framework $WORK/macos/{{.Output}}.framework -output {{.Output}}.xcframework
`))

func TestBindIOSAll(t *testing.T) {
	if !xcodeAvailable() {
		t.Skip("Xcode is missing")
	}
	defer func() {
		xout = os.Stderr
		buildN = false
		buildX = false
		buildO = ""
		buildTarget = ""
		bindPrefix = ""
	}()
	buildN = true
	buildX = true
	buildO = "Asset.xcframework"
	buildTarget = "ios"

	buf := new(bytes.Buffer)
	xout = buf
	gopath = filepath.SplitList(goEnv("GOPATH"))[0]
	if goos == "windows" {
		os.Setenv("HOMEDRIVE", "C:")
	}
	cmdBind.flag.Parse([]string{"golang.org/x/mobile/asset"})
	if err := runBind(cmdBind); err != nil {
		t.Log(buf.String())
		t.Fatal(err)
	}
}

func TestBindWithGoModules(t *testing.T) {
	if runtime.GOOS == "android" {
		t.Skipf("gomobile and gobind are not available on %s", runtime.GOOS)
	}

	dir, err := ioutil.TempDir("", "gomobile-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	if out, err := exec.Command("go", "build", "-o="+dir, "golang.org/x/mobile/cmd/gobind").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, string(out))
	}
	if out, err := exec.Command("go", "build", "-o="+dir, "golang.org/x/mobile/cmd/gomobile").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, string(out))
	}
	path := dir
	if p := os.Getenv("PATH"); p != "" {
		path += string(filepath.ListSeparator) + p
	}

	for _, target := range []string{"android", "ios"} {
		t.Run(target, func(t *testing.T) {
			switch target {
			case "android":
				androidHome := os.Getenv("ANDROID_HOME")
				if androidHome == "" {
					t.Skip("ANDROID_HOME not found, skipping bind")
				}
				if _, err := androidAPIPath(); err != nil {
					t.Skip("No android API platform found in $ANDROID_HOME, skipping bind")
				}
			case "ios":
				if !xcodeAvailable() {
					t.Skip("Xcode is missing")
				}
			}

			var out string
			switch target {
			case "android":
				out = filepath.Join(dir, "cgopkg.aar")
			case "ios":
				out = filepath.Join(dir, "Cgopkg.xcframework")
			}

			tests := []struct {
				Name string
				Path string
				Dir  string
			}{
				{
					Name: "Absolute Path",
					Path: "golang.org/x/mobile/bind/testdata/cgopkg",
				},
				{
					Name: "Relative Path",
					Path: "./bind/testdata/cgopkg",
					Dir:  filepath.Join("..", ".."),
				},
			}

			for _, tc := range tests {
				tc := tc
				t.Run(tc.Name, func(t *testing.T) {
					cmd := exec.Command(filepath.Join(dir, "gomobile"), "bind", "-target="+target, "-o="+out, tc.Path)
					cmd.Env = append(os.Environ(), "PATH="+path, "GO111MODULE=on")
					cmd.Dir = tc.Dir
					if out, err := cmd.CombinedOutput(); err != nil {
						t.Errorf("gomobile bind failed: %v\n%s", err, string(out))
					}
				})
			}
		})
	}
}
