## Local notes for this machine

Read this section before the first REA call. It is short on purpose; the rest of the skill is reference.

### Call shapes

Use exactly these argument names. An `invalid_request` names the wrong field in `details.issues`: fix that field, never resend the same call.

- `open_binary {"path": "/absolute/file"}`. One target per connection: opening another target closes the current one. Ghidra checkpoints are on: the first open of a file analyses it and saves the result, later opens of the same file restore it with your renames and comments, `close_binary` saves new ones. If the open fails with "another REA session owns this checkpoint", a parallel session has that file open: tell the user instead of retrying.
- `search_procedures {"pattern": "name"}`
- `procedure_pseudo_code {"procedure": "name or 0xADDR"}`
- `procedure_assembly {"procedure": "name or 0xADDR"}`, no `detail` argument.
- `read_bytes {"address": "0xADDR", "length": 64}`
- `xrefs {"address": "0xADDR"}`
- `address_to_file_offset {"address": "0xADDR"}`
- `inspect_with_objdump {"path": "/absolute/file", "operation": "disassemble", "symbol": "name"}`: GNU objdump for every architecture. The `objdump` on PATH is x86 only.
- `inspect_analysis_view {"source": {"kind": "retained-evidence", "evidence_id": "<parent_evidence_id>"}, "view": {"kind": "summary"}}`. Pseudocode pages: `"view": {"kind": "native", "facet": "pseudocode", "offset": 0, "limit": 128}`. Its schema only says `object`; the shapes are in its description, do not invent others.

### Compacted results

A local hook shortens parts of REA results that this agent has already seen:

- `"same_as": {"evidence_id": "X", "fields": [...]}`: those fields are identical to the ones in evidence X, shown earlier.
- `"subject": "same as in X"`, `"analysis_profile": "same as in X"`: the same, for one field.
- `"limitations_shown_earlier": N`: N limitations were already shown earlier in this conversation and are left out. The new ones are still listed.
- `"raw_result": "same as normalized_result"`: both were identical.
- Limitations that differ only in one address are merged into one line with a list: `terminal call at [0x1026fa, 0x1027a1] to 0x1022e0 ...`.
- Multi-line text results (pseudocode, assembly) come as plain text after the JSON, under a `normalized_result:` line; inside the JSON `normalized_result` then reads "shown below the JSON".

### Limitations

- "Known provider defect ghidra-NNNN" lines are attached to every Ghidra 12.2 result. They do not mean this target is affected: ghidra-10154 is AArch64 only, ghidra-9996 is clang `-O0` x86-64 switches.
- "terminal call at A to B" means Ghidra treats B as no-return and ends the function after A. If the code after A is real (stores, more calls), the flag is likely wrong: read that tail with `procedure_assembly` or `inspect_with_objdump`, not in pseudocode.

### Addresses

- Ghidra addresses are authoritative inside REA. A PIE ELF (x86-64, AArch64) is loaded at image base `0x100000`: objdump shows the same instruction at the Ghidra address minus `0x100000`. A PE uses the ImageBase from its header, which is 0 for many UEFI modules.
- Convert with `address_to_file_offset` instead of doing arithmetic. Never add or subtract hex addresses in your head: run it in Bash, `printf '0x%x\n' $((0x165000 - 0x100000))`, and check the result against the section end the same way.
- Never compare addresses with another disassembler's output before converting both to the same base.

### Pseudocode versus assembly

- Pseudocode carries `extraout_*` variables and may show a global as `SYMBOL.field + 0xNNN` (with image base 0 the symbol is often the DOS header). Take operation order, constants and global addresses from `procedure_assembly`.
- `read_bytes` returns only bytes backed by the file. Past a section's file size (`.bss`, `VirtualSize > SizeOfRawData`) there are no bytes: `returned_bytes: 0, complete: false` is the answer, not a failure to retry.

### What is verified

On Ghidra 12.2 with AArch64 (`hello` and `bash` compared with objdump), every `adrp` page and every direct branch target matched once the base was subtracted. Do not re-verify those. The weak spot is `switch` jump tables (`br xN` after a table load): Ghidra 12.2 may not recover them, and then the case code is missing or shown as a call. Check those against `inspect_with_objdump`, or for an address range in a stripped binary run `nix shell nixpkgs#llvm -c llvm-objdump -d --start-address=0xA --stop-address=0xB /absolute/file` (file addresses, without the Ghidra base).
