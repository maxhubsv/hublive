@echo off
echo === Building HubLive Screen Agent ===
echo.

set CLANG_CL=f:\webrtc-checkout\src\third_party\llvm-build\Release+Asserts\bin\clang-cl.exe
set RC_COMPILER=C:\Program Files (x86)\Windows Kits\10\bin\10.0.26100.0\x64\rc.exe
set MT_TOOL=C:\Program Files (x86)\Windows Kits\10\bin\10.0.26100.0\x64\mt.exe

if not exist build (
    echo Configuring CMake with clang-cl...
    cmake -B build -G Ninja -DCMAKE_C_COMPILER="%CLANG_CL%" -DCMAKE_CXX_COMPILER="%CLANG_CL%" -DCMAKE_LINKER_TYPE=LLD -DCMAKE_RC_COMPILER="%RC_COMPILER%" -DCMAKE_MT="%MT_TOOL%"
    if errorlevel 1 (
        echo CMake configure failed!
        exit /b 1
    )
)

echo Building...
cmake --build build
if errorlevel 1 (
    echo Build failed!
    exit /b 1
)

echo.
echo === Build complete ===
echo Output: build\screen_agent.exe
echo.
echo To run: copy config.yaml to build\ and run screen_agent.exe
