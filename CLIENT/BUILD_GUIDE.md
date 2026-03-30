# Build Guide — HubLive Screen Agent

## Yêu cầu

- **Windows 10+ x64**
- **CMake** >= 3.20
- **Ninja** build system
- **libwebrtc** pre-built (xem phần dưới)

## libwebrtc Location

```
f:\webrtc-checkout\
  src\
    out\Release\          ← build output (.lib files)
    out\Release\args.gn   ← build config
    third_party\llvm-build\Release+Asserts\bin\  ← clang-cl compiler
    buildtools\win\gn.exe  ← GN build tool
```

## Build libwebrtc (Release)

### args.gn (`f:\webrtc-checkout\src\out\Release\args.gn`)

```gn
is_debug = false
is_component_build = false
rtc_include_tests = false
treat_warnings_as_errors = false
rtc_enable_protobuf = true
enable_iterator_debugging = false
dcheck_always_on = true
symbol_level = 0
target_os = "win"
target_cpu = "x64"
use_lld = true
rtc_build_tools = false
rtc_build_examples = false
```

### Build commands

```bash
cd f:\webrtc-checkout\src

# Regenerate build files (set env to use local MSVC toolchain)
set DEPOT_TOOLS_WIN_TOOLCHAIN=0
buildtools\win\gn.exe gen out/Release

# Build (incremental — fast after first build)
ninja -C out/Release webrtc
```

Output: `f:\webrtc-checkout\src\out\Release\obj\webrtc.lib` (~24MB exe khi link)

### Key args.gn notes

| Flag | Value | Why |
|------|-------|-----|
| `is_debug` | false | Release optimized build, no verbose logging |
| `dcheck_always_on` | true | Keep runtime check symbols (needed by agent linking) |
| `rtc_include_tests` | false | Skip test code (faster build) |
| `rtc_build_tools` | false | Skip CLI tools |
| `rtc_build_examples` | false | Skip examples |
| `symbol_level` | 0 | Minimal symbols (smaller output) |

## Build Agent

```bash
cd CLIENT

# First time (configure CMake)
cmake -B build -G Ninja ^
  -DCMAKE_C_COMPILER="f:/webrtc-checkout/src/third_party/llvm-build/Release+Asserts/bin/clang-cl.exe" ^
  -DCMAKE_CXX_COMPILER="f:/webrtc-checkout/src/third_party/llvm-build/Release+Asserts/bin/clang-cl.exe" ^
  -DCMAKE_LINKER_TYPE=LLD ^
  -DCMAKE_RC_COMPILER="C:/Program Files (x86)/Windows Kits/10/bin/10.0.26100.0/x64/rc.exe" ^
  -DCMAKE_MT="C:/Program Files (x86)/Windows Kits/10/bin/10.0.26100.0/x64/mt.exe"

# Build
cmake --build build

# Output
build\screen_agent.exe  (~ 24MB)
```

### Rebuild (sau lần đầu)

```bash
cmake --build build
```

Chỉ compile lại files thay đổi. Nếu thêm file .cc mới vào CMakeLists.txt, xóa `build/` và configure lại.

## Run

```bash
copy config.yaml build\
build\screen_agent.exe
```

Hoặc double-click `screen_agent.exe` (config.yaml tự resolve từ exe directory).

## Important

- **Compiler**: PHẢI dùng clang-cl từ libwebrtc build tree. MSVC ABI không tương thích.
- **CRT**: Release build dùng `/MT` (static CRT). CMakeLists.txt set `MultiThreaded`.
- **RTTI**: Disabled (`/GR-`) để match libwebrtc.
- **libc++**: Dùng libc++ từ libwebrtc, không phải MSVC STL.
