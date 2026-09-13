/**
 * Packs `dist/` into `mdn-extension.zip`.
 *
 * Written by hand rather than shelling out to `zip`, which is not installed
 * everywhere the repository builds, or pulling in a dependency for eighty
 * lines of format. The output is deterministic: entries in sorted order with a
 * fixed timestamp, so an unchanged build produces an identical zip.
 */

import { deflateRawSync } from "node:zlib";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const source = path.join(root, "dist");
const target = path.join(root, "mdn-extension.zip");

/** 1980-01-01 00:00, the earliest a DOS timestamp can express. */
const DOS_TIME = 0;
const DOS_DATE = (0 << 9) | (1 << 5) | 1;

const CRC_TABLE = (() => {
  const table = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    table[n] = c >>> 0;
  }
  return table;
})();

function crc32(buf) {
  let c = 0xffffffff;
  for (const b of buf) c = CRC_TABLE[(c ^ b) & 0xff] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}

function filesUnder(dir, prefix = "") {
  const out = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true }).sort((a, b) =>
    a.name < b.name ? -1 : 1,
  )) {
    const name = prefix === "" ? entry.name : `${prefix}/${entry.name}`;
    if (entry.isDirectory()) out.push(...filesUnder(path.join(dir, entry.name), name));
    else out.push(name);
  }
  return out;
}

if (!fs.existsSync(path.join(source, "manifest.json"))) {
  console.error(`no build to pack: ${source} has no manifest.json (run the build first)`);
  process.exit(1);
}

const parts = [];
const central = [];
let offset = 0;

for (const name of filesUnder(source)) {
  const raw = fs.readFileSync(path.join(source, name));
  const compressed = deflateRawSync(raw, { level: 9 });
  const nameBytes = Buffer.from(name, "utf8");
  const crc = crc32(raw);

  const local = Buffer.alloc(30);
  local.writeUInt32LE(0x04034b50, 0);
  local.writeUInt16LE(20, 4); // version needed
  local.writeUInt16LE(0x0800, 6); // UTF-8 names
  local.writeUInt16LE(8, 8); // deflate
  local.writeUInt16LE(DOS_TIME, 10);
  local.writeUInt16LE(DOS_DATE, 12);
  local.writeUInt32LE(crc, 14);
  local.writeUInt32LE(compressed.length, 18);
  local.writeUInt32LE(raw.length, 22);
  local.writeUInt16LE(nameBytes.length, 26);
  parts.push(local, nameBytes, compressed);

  const entry = Buffer.alloc(46);
  entry.writeUInt32LE(0x02014b50, 0);
  entry.writeUInt16LE(0x031e, 4); // made by: Unix, spec 3.0
  entry.writeUInt16LE(20, 6);
  entry.writeUInt16LE(0x0800, 8);
  entry.writeUInt16LE(8, 10);
  entry.writeUInt16LE(DOS_TIME, 12);
  entry.writeUInt16LE(DOS_DATE, 14);
  entry.writeUInt32LE(crc, 16);
  entry.writeUInt32LE(compressed.length, 20);
  entry.writeUInt32LE(raw.length, 24);
  entry.writeUInt16LE(nameBytes.length, 28);
  entry.writeUInt32LE(((0o100644 << 16) >>> 0), 38); // external attributes: a regular file
  entry.writeUInt32LE(offset, 42);
  central.push(entry, nameBytes);

  offset += local.length + nameBytes.length + compressed.length;
}

const directory = Buffer.concat(central);
const end = Buffer.alloc(22);
const count = central.length / 2;
end.writeUInt32LE(0x06054b50, 0);
end.writeUInt16LE(count, 8);
end.writeUInt16LE(count, 10);
end.writeUInt32LE(directory.length, 12);
end.writeUInt32LE(offset, 16);

fs.writeFileSync(target, Buffer.concat([...parts, directory, end]));
console.log(`${path.relative(root, target)}: ${count} files, ${fs.statSync(target).size} bytes`);
