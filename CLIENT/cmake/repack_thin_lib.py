#!/usr/bin/env python3
"""Repack a GNU thin archive into a real MSVC static library.

Usage: python repack_thin_lib.py <thin_archive.lib> <output.lib> <lib_exe_path>

Parses the thin archive to find referenced .obj files, then invokes MSVC
lib.exe to create a proper COFF static library from those .obj files.
"""

import os
import subprocess
import sys
import tempfile


def parse_thin_archive(path):
    """Parse a GNU thin archive and return relative .obj member paths."""
    members = []
    try:
        with open(path, "rb") as f:
            sig = f.read(8)
            if sig != b"!<thin>\n":
                print(f"ERROR: {path} is not a thin archive", file=sys.stderr)
                return []
            data = f.read()
    except (IOError, OSError) as e:
        print(f"ERROR: Cannot read {path}: {e}", file=sys.stderr)
        return []

    text = data.decode("latin-1")

    # Find the long names section (// header)
    idx = text.find("//")
    if idx < 0:
        return []

    # Parse archive header: name(16) date(12) uid(6) gid(6) mode(8) size(10) end(2)
    header = text[idx : idx + 60]
    try:
        size = int(header[48:58].strip())
    except ValueError:
        return []

    names_start = idx + 60
    names_section = text[names_start : names_start + size]

    # Names are separated by /\n
    for name in names_section.split("/\n"):
        name = name.strip()
        if name and name.endswith(".obj"):
            members.append(name)

    return members


def main():
    if len(sys.argv) != 4:
        print(
            f"Usage: {sys.argv[0]} <thin_archive.lib> <output.lib> <lib_exe_path>",
            file=sys.stderr,
        )
        sys.exit(1)

    thin_lib = sys.argv[1]
    output_lib = sys.argv[2]
    lib_exe = sys.argv[3]

    members = parse_thin_archive(thin_lib)
    if not members:
        print(f"ERROR: No .obj members found in {thin_lib}", file=sys.stderr)
        sys.exit(1)

    lib_dir = os.path.dirname(thin_lib)
    abs_paths = []
    for m in members:
        full = os.path.normpath(os.path.join(lib_dir, m))
        if os.path.isfile(full):
            abs_paths.append(full)
        else:
            print(f"WARNING: Missing obj file: {full}", file=sys.stderr)

    if not abs_paths:
        print("ERROR: No valid .obj files found", file=sys.stderr)
        sys.exit(1)

    # Write response file for lib.exe
    rsp_path = output_lib + ".rsp"
    with open(rsp_path, "w") as f:
        for p in abs_paths:
            f.write('"' + p.replace("\\", "/") + '"\n')

    # Run lib.exe
    cmd = [lib_exe, "/NOLOGO", f"/OUT:{output_lib}", f"@{rsp_path}"]
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        print(f"ERROR: lib.exe failed: {result.stderr}", file=sys.stderr)
        sys.exit(1)

    print(f"Created {output_lib} with {len(abs_paths)} obj files")


if __name__ == "__main__":
    main()
