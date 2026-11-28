import re
import struct
import sys

CLI_ORIG = re.compile(rb"\x48\x85\xc0\x0f\x84....\x80\x78\x08\x00\x0f\x85....", re.DOTALL)
CLI_DONE = re.compile(rb"\x48\x85\xc0\x0f\x84....\x90\x90\x90\x90\x0f\x85....", re.DOTALL)
BACKEND_ORIG = re.compile(
    rb"\x48\x85\xc0\x0f\x85....\x48\x85\xc9\x0f\x84....\x80\x79\x08\x00\x0f\x85...."
    rb"\x48\x89\xc8\x90\xe8....",
    re.DOTALL,
)
BACKEND_DONE = re.compile(
    rb"\x48\x85\xc0\x0f\x85....\x48\x85\xc9\x0f\x84....\x90\x90\x90\x90\x0f\x85...."
    rb"\x48\x89\xc8\x90\xe8....",
    re.DOTALL,
)
HUB_ORIG = re.compile(rb"\x80\x78\x08\x00\x74.\x48\x8b.\x24.\x48\x89.\x60", re.DOTALL)
HUB_DONE = re.compile(rb"\xc6\x40\x08\x01\x90\x90\x48\x8b.\x24.\x48\x89.\x60", re.DOTALL)
CLI_FIX = b"\x90\x90\x90\x90"
CLI_AT = 9
BACKEND_FIX = b"\x90\x90\x90\x90"
BACKEND_AT = 18
HUB_FIX = b"\xc6\x40\x08\x01\x90\x90"


def executable_sections(data):
    if data[:4] != b"\x7fELF":
        raise SystemExit("not an ELF file")
    if data[4] != 2:
        raise SystemExit("not ELF64")
    if struct.unpack_from("<H", data, 0x12)[0] != 0x3E:
        raise SystemExit("not x86_64")
    shoff = struct.unpack_from("<Q", data, 0x28)[0]
    shentsize = struct.unpack_from("<H", data, 0x3A)[0]
    shnum = struct.unpack_from("<H", data, 0x3C)[0]
    out = []
    for index in range(shnum):
        base = shoff + index * shentsize
        sh_type = struct.unpack_from("<I", data, base + 4)[0]
        sh_flags = struct.unpack_from("<Q", data, base + 8)[0]
        if sh_type == 1 and sh_flags & 0x4:
            out.append(
                (
                    struct.unpack_from("<Q", data, base + 0x18)[0],
                    struct.unpack_from("<Q", data, base + 0x20)[0],
                )
            )
    return out


def main(kind, path):
    with open(path, "rb") as handle:
        data = bytearray(handle.read())
    backend = []
    login = []
    hub = []
    for offset, size in executable_sections(data):
        blob = bytes(data[offset : offset + size])
        for match in BACKEND_ORIG.finditer(blob):
            je = struct.unpack_from("<i", blob, match.start() + 14)[0]
            jne = struct.unpack_from("<i", blob, match.start() + 24)[0]
            if je != jne + 10:
                raise SystemExit(
                    f"backend gate branches diverge at {offset + match.start():#x}"
                )
            backend.append((offset + match.start(), False))
        for match in BACKEND_DONE.finditer(blob):
            backend.append((offset + match.start(), True))
        for match in CLI_ORIG.finditer(blob):
            je = struct.unpack_from("<i", blob, match.start() + 5)[0]
            jne = struct.unpack_from("<i", blob, match.start() + 15)[0]
            if je != jne + 10:
                raise SystemExit(
                    f"login gate branches diverge at {offset + match.start():#x}"
                )
            login.append((offset + match.start(), False))
        for match in CLI_DONE.finditer(blob):
            login.append((offset + match.start(), True))
        for match in HUB_ORIG.finditer(blob):
            hub.append((offset + match.start(), False))
        for match in HUB_DONE.finditer(blob):
            hub.append((offset + match.start(), True))
    if kind == "agy":
        if len(hub) > 1:
            raise SystemExit(f"agy: {len(hub)} hub gates, expected 0..1")
        if backend:
            if len(backend) != 1:
                raise SystemExit(f"agy: {len(backend)} backend gates, expected 1")
            gates, fix, at, profile = backend, BACKEND_FIX, BACKEND_AT, "backend"
        else:
            if not 1 <= len(login) <= 2:
                raise SystemExit(f"agy: {len(login)} login gates, expected 1..2")
            gates, fix, at, profile = login, CLI_FIX, CLI_AT, "login"
    else:
        if len(hub) != 1:
            raise SystemExit(f"hub: {len(hub)} hub gates, expected 1")
        gates, fix, at, profile = hub, HUB_FIX, 0, "hub"
    changes = 0
    for gate_at, done in gates:
        if not done:
            data[gate_at + at : gate_at + at + len(fix)] = fix
            changes += 1
    if kind == "agy":
        for gate_at, done in hub:
            if not done:
                data[gate_at : gate_at + len(HUB_FIX)] = HUB_FIX
                changes += 1
    if changes:
        with open(path, "wb") as handle:
            handle.write(data)
    print(
        f"{kind}: {changes} gates patched, profile {profile}, "
        f"{len(login)} login, {len(backend)} backend, {len(hub)} hub"
    )


main(sys.argv[1], sys.argv[2])
