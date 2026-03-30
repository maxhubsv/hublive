cmake_minimum_required(VERSION 3.20)
project(hublive_screen_agent LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# --------------------------------------------------------------------------
# Paths to libwebrtc (built with GN + Clang, default debug config)
# --------------------------------------------------------------------------
set(WEBRTC_SRC "f:/webrtc-checkout/src")
set(WEBRTC_OUT "${WEBRTC_SRC}/out/Release")
set(WEBRTC_OBJ "${WEBRTC_OUT}/obj")

# --------------------------------------------------------------------------
# Toolchain: use the same clang-cl / lld-link that built libwebrtc.
# --------------------------------------------------------------------------
# These paths are intentionally absolute so that Ninja finds them regardless
# of PATH.  When using Ninja, cmake must be configured with:
#   cmake -B build -G Ninja \
#     -DCMAKE_C_COMPILER="f:/webrtc-checkout/src/third_party/llvm-build/Release+Asserts/bin/clang-cl.exe" \
#     -DCMAKE_CXX_COMPILER="f:/webrtc-checkout/src/third_party/llvm-build/Release+Asserts/bin/clang-cl.exe" \
#     -DCMAKE_LINKER_TYPE=LLD
set(LLVM_DIR "${WEBRTC_SRC}/third_party/llvm-build/Release+Asserts")

# --------------------------------------------------------------------------
# libc++ paths (must come BEFORE MSVC STL headers)
#
# The webrtc GN build uses:
#   -I../../buildtools/third_party/libc++        (__config_site + __assertion_handler)
#   -I../../third_party/libc++/src/include        (main libc++ headers)
# These are plain -I flags that precede the -imsvc flags for MSVC headers,
# so libc++ wins over <string>, <vector>, etc.
# --------------------------------------------------------------------------
set(LIBCXX_BUILDTOOLS "${WEBRTC_SRC}/buildtools/third_party/libc++")
set(LIBCXX_HEADERS    "${WEBRTC_SRC}/third_party/libc++/src/include")

# libc++ .obj files produced by the webrtc build (no .lib archive exists)
set(LIBCXX_OBJ_DIR "${WEBRTC_OBJ}/buildtools/third_party/libc++/libc++")

# Clang runtime builtins (same LLVM build)
set(CLANG_RT_BUILTINS "${LLVM_DIR}/lib/clang/23/lib/windows/clang_rt.builtins-x86_64.lib")

# --------------------------------------------------------------------------
# Protobuf code generation from .proto files
# --------------------------------------------------------------------------
set(PROTOC_EXE "${WEBRTC_OUT}/protoc.exe")
set(PROTO_SRC_DIR "${CMAKE_SOURCE_DIR}/proto")
set(PROTO_GEN_DIR "${CMAKE_BINARY_DIR}/proto_gen")
file(MAKE_DIRECTORY ${PROTO_GEN_DIR})

set(PROTO_FILES
    "${PROTO_SRC_DIR}/hublive_models.proto"
    "${PROTO_SRC_DIR}/hublive_rtc.proto"
)

set(PROTO_GEN_SRCS "")
set(PROTO_GEN_HDRS "")
foreach(PROTO_FILE ${PROTO_FILES})
    get_filename_component(PROTO_NAME ${PROTO_FILE} NAME_WE)
    list(APPEND PROTO_GEN_SRCS "${PROTO_GEN_DIR}/${PROTO_NAME}.pb.cc")
    list(APPEND PROTO_GEN_HDRS "${PROTO_GEN_DIR}/${PROTO_NAME}.pb.h")
    add_custom_command(
        OUTPUT "${PROTO_GEN_DIR}/${PROTO_NAME}.pb.cc" "${PROTO_GEN_DIR}/${PROTO_NAME}.pb.h"
        COMMAND ${PROTOC_EXE}
            --proto_path=${PROTO_SRC_DIR}
            --cpp_out=${PROTO_GEN_DIR}
            ${PROTO_FILE}
        DEPENDS ${PROTO_FILE}
        COMMENT "Generating protobuf C++ for ${PROTO_NAME}.proto"
    )
endforeach()

# Custom target to trigger proto generation
add_custom_target(generate_protos DEPENDS ${PROTO_GEN_SRCS} ${PROTO_GEN_HDRS})

# --------------------------------------------------------------------------
# Collect all libc++ object files built by GN.
# They must be linked into our executable because webrtc.lib does NOT
# bundle them (GN links them as loose .obj references).
# --------------------------------------------------------------------------
file(GLOB LIBCXX_OBJS "${LIBCXX_OBJ_DIR}/*.obj")
if(NOT LIBCXX_OBJS)
    message(FATAL_ERROR "No libc++ .obj files found in ${LIBCXX_OBJ_DIR}")
endif()

# --------------------------------------------------------------------------
# Source files  (protobuf .pb.cc included — now ABI-compatible)
# --------------------------------------------------------------------------
set(SOURCES
    src/main.cc
    src/config.cc
    src/jwt_token.cc
    src/websocket_transport.cc
    src/signaling_client.cc
    src/screen_capture_source.cc
    src/peer_connection_agent.cc
    src/whip_client.cc
    src/input_injector.cc
    src/audio_capture.cc
    src/audio_resampler.cc
    src/audio_mixer.cc
    src/custom_audio_source.cc
    ${PROTO_GEN_SRCS}
)

add_executable(screen_agent ${SOURCES})
add_dependencies(screen_agent generate_protos)

# --------------------------------------------------------------------------
# Include directories
#
# BEFORE + SYSTEM ensures libc++ headers are searched before MSVC STL.
# This is equivalent to the -I flags that the webrtc build places ahead
# of the -imsvc flags.
# --------------------------------------------------------------------------
target_include_directories(screen_agent BEFORE PRIVATE
    ${LIBCXX_BUILDTOOLS}
    ${LIBCXX_HEADERS}
)

target_include_directories(screen_agent PRIVATE
    ${CMAKE_SOURCE_DIR}/src
    ${PROTO_GEN_DIR}
    ${WEBRTC_SRC}
    ${WEBRTC_SRC}/third_party/abseil-cpp
    ${WEBRTC_SRC}/third_party/protobuf/src
    ${WEBRTC_SRC}/third_party/boringssl/src/include
    ${WEBRTC_SRC}/third_party/libyuv/include
    ${WEBRTC_OUT}/gen
)

# --------------------------------------------------------------------------
# Compile definitions matching libwebrtc build
# --------------------------------------------------------------------------
target_compile_definitions(screen_agent PRIVATE
    WEBRTC_WIN
    WIN32_LEAN_AND_MEAN
    NOMINMAX
    UNICODE
    _UNICODE
    _CRT_SECURE_NO_WARNINGS
    WEBRTC_USE_H264
    RTC_ENABLE_VP9
    _SILENCE_ALL_CXX17_DEPRECATION_WARNINGS
    GOOGLE_PROTOBUF_NO_RTTI
    # libc++ defines that the webrtc build injects via -D flags
    _LIBCPP_HARDENING_MODE=_LIBCPP_HARDENING_MODE_EXTENSIVE
    _LIBCPP_DISABLE_VISIBILITY_ANNOTATIONS
    _LIBCPP_INSTRUMENTED_WITH_ASAN=0
    WEBRTC_ENABLE_PROTOBUF=1
)

# --------------------------------------------------------------------------
# Linking
# --------------------------------------------------------------------------
set(WEBRTC_LIB "${WEBRTC_OBJ}/webrtc.lib")

# Additional WebRTC component libraries not bundled in webrtc.lib.
# The GN build produces hundreds of small .lib archives; we glob them all
# and let the linker pull in only what is actually referenced.
file(GLOB_RECURSE WEBRTC_COMPONENT_LIBS "${WEBRTC_OBJ}/*.lib")
# Remove webrtc.lib itself (already linked explicitly) and protobuf libs
# to avoid duplicate symbols.
list(FILTER WEBRTC_COMPONENT_LIBS EXCLUDE REGEX "webrtc\\.lib$")
list(FILTER WEBRTC_COMPONENT_LIBS EXCLUDE REGEX "protobuf_full\\.lib$")
list(FILTER WEBRTC_COMPONENT_LIBS EXCLUDE REGEX "protobuf_lite\\.lib$")
# Exclude test-only libs to avoid pulling in test code.
list(FILTER WEBRTC_COMPONENT_LIBS EXCLUDE REGEX "/test/")
list(FILTER WEBRTC_COMPONENT_LIBS EXCLUDE REGEX "/testing/")
list(FILTER WEBRTC_COMPONENT_LIBS EXCLUDE REGEX "_test\\.lib$")
list(FILTER WEBRTC_COMPONENT_LIBS EXCLUDE REGEX "_tests\\.lib$")
list(FILTER WEBRTC_COMPONENT_LIBS EXCLUDE REGEX "_unittest\\.lib$")
list(FILTER WEBRTC_COMPONENT_LIBS EXCLUDE REGEX "_unittests\\.lib$")
list(FILTER WEBRTC_COMPONENT_LIBS EXCLUDE REGEX "mock_\\.lib$")
list(FILTER WEBRTC_COMPONENT_LIBS EXCLUDE REGEX "fake_\\.lib$")

# Protobuf libraries (thin archives readable by lld-link)
set(PROTOBUF_FULL_LIB "${WEBRTC_OBJ}/third_party/protobuf/protobuf_full.lib")
set(PROTOBUF_LITE_LIB "${WEBRTC_OBJ}/third_party/protobuf/protobuf_lite.lib")

target_link_libraries(screen_agent PRIVATE
    ${WEBRTC_LIB}
    ${WEBRTC_COMPONENT_LIBS}
    ${PROTOBUF_FULL_LIB}
    ${PROTOBUF_LITE_LIB}
    # libc++ object files (ABI-compatible with webrtc.lib)
    ${LIBCXX_OBJS}
    # Clang runtime builtins
    ${CLANG_RT_BUILTINS}
    # Windows system libraries
    winhttp.lib
    ws2_32.lib
    secur32.lib
    crypt32.lib
    iphlpapi.lib
    msdmo.lib
    dmoguids.lib
    wmcodecdspuuid.lib
    amstrmid.lib
    strmiids.lib
    d3d11.lib
    dxgi.lib
    dwmapi.lib
    shcore.lib
    winmm.lib
    ole32.lib
    oleaut32.lib
    uuid.lib
    user32.lib
    gdi32.lib
    advapi32.lib
    shell32.lib
    comdlg32.lib
    shlwapi.lib
)

# --------------------------------------------------------------------------
# Compiler and linker flags
# --------------------------------------------------------------------------
# Match the CRT used by the webrtc build (/MTd = debug static CRT).
set_property(TARGET screen_agent PROPERTY MSVC_RUNTIME_LIBRARY "MultiThreadedDebug")

# Disable RTTI to match libwebrtc (/GR-). Keep C++ exceptions (/EHsc).
target_compile_options(screen_agent PRIVATE /GR- /EHsc /W3)

# Linker flags matching webrtc build
target_link_options(screen_agent PRIVATE
    /NODEFAULTLIB:libcmt.lib
    /DEFAULTLIB:libcpmtd.lib
)
