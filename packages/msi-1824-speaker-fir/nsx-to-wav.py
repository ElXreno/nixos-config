import base64
import struct
import sys
import xml.etree.ElementTree as ET

src, out = sys.argv[1], sys.argv[2]
root = ET.parse(src).getroot()


def taps(name: str) -> list[float]:
    text = root.findtext(f"Settings/{name}/Value")
    if text is None:
        sys.exit(f"{name} not found in {src}")
    data = base64.b64decode(text.strip())
    return list(struct.unpack(f"<{len(data) // 4}f", data))


fl = taps("kSet_DeviceOptimizationFilterFL")
fr = taps("kSet_DeviceOptimizationFilterFR")
if len(fl) != len(fr) or not fl:
    sys.exit(f"unexpected FIR lengths {len(fl)} and {len(fr)}")

rate = 48000
frames = b"".join(struct.pack("<ff", left, right) for left, right in zip(fl, fr))
header = (
    b"RIFF"
    + struct.pack("<I", 36 + len(frames))
    + b"WAVE"
    + b"fmt "
    + struct.pack("<IHHIIHH", 16, 3, 2, rate, rate * 8, 8, 32)
    + b"data"
    + struct.pack("<I", len(frames))
)
with open(out, "wb") as f:
    _ = f.write(header + frames)
