# GoLSM — A Log-Structured Key-Value Storage Engine

GoLSM is a lightweight persistent key-value store written from scratch in Go, built to explore the internals of **log-structured merge (LSM) storage engines**.

I wrote up the design and build process in more detail here: [Stratum: A KV Store in Go](https://stratum.hashnode.dev/stratum-a-kv-store-in-go)

Instead of keeping the whole database in memory, GoLSM keeps only recent writes in RAM and persists everything else to disk. It's an educational implementation — not for production — meant to demonstrate the core ideas behind systems like LevelDB/RocksDB/Cassandra's storage layer.

---

## Why not just append everything to a log?

A naive persistent KV store just appends every operation to a file and rebuilds state on startup by:
1. Reading the entire log from disk
2. Parsing every operation
3. Hashing every key
4. Reconstructing the full in-memory table

This works for small datasets but gets expensive fast as the number of keys grows into the millions. GoLSM avoids requiring the whole DB to live in memory by splitting storage into two zones:

```
RAM
└── MemTable — recent writes

DISK
├── Write-Ahead Log — crash recovery for the MemTable
├── SSTable 1
├── SSTable 2
├── SSTable 3 ...  — persisted database state
```

Recent writes go into an in-memory hash table (MemTable). Once enough writes accumulate, the MemTable is sorted and flushed to disk as an immutable **SSTable**.

---

## Architecture

```
PUT / DELETE
     ↓
Write-Ahead Log (data.log)
     ↓
MemTable (in-memory hash table)
     ↓
[flush threshold reached]
     ↓
Sort entries by key
     ↓
SSTable N (sorted key-values)
     ↓
Sparse Index (key → byte offset)
```

Four core components, each solving a different storage problem:
- **Write-Ahead Log (WAL)** — durability
- **MemTable** — fast recent writes
- **SSTables** — durable sorted storage
- **Sparse Indexes** — fast lookups without scanning whole files

---

## Write Path

```
PUT(key, value)
  → Append operation to WAL
  → Update MemTable
  → Increment WAL operation count
  → Check flush threshold
       ├── below threshold → continue normally
       └── threshold reached:
            → Collect MemTable entries
            → Sort by key
            → Write immutable SSTable
            → Generate sparse index
            → Sync files to disk
            → Truncate WAL
            → Clear MemTable
```

**Key detail:** the WAL is written *before* the MemTable is updated. This ordering matters — if the process crashes between the WAL write and the MemTable update completing, the operation is still recoverable on disk and gets replayed on restart.

---

## Write-Ahead Log (WAL)

The WAL provides durability for whatever's currently sitting in the MemTable (which lives only in RAM). Stored in `data.log`, operations are appended sequentially:

```
PUT apple red
PUT banana yellow
PUT apple green
DELETE banana
```

The WAL logs *operations*, not final state — so multiple versions of the same key can exist in it. E.g. `PUT user Alice`, `PUT user Bob`, `PUT user Charlie` → current value is `Charlie`, but all three entries remain in the WAL until the MemTable is flushed.

**Why it exists:** without a WAL, a crash after writing to the MemTable (RAM-only) loses that data entirely. With a WAL, a crash is recoverable — on restart, GoLSM replays `data.log` to rebuild the MemTable.

---

## MemTable

Stores recent writes in memory, implemented as a **hash table with separate chaining**:

```
table
├── Bucket 0 → Entry, Entry
├── Bucket 1 → Entry
├── Bucket 2 → ...
```

Each entry holds a `Key` + `Value`. Hashing uses **FNV-1a (64-bit)**, with bucket selection via `hash(key) % BUCKET_SIZE`.

**PUT logic:**
1. Hash the key
2. Select the bucket
3. Search the bucket for the key
4. If found, update the value
5. Otherwise, append a new entry

This collapses repeated updates to the same key within the MemTable — e.g. `PUT apple 1/2/3` in the WAL results in a single `apple → 3` entry in the MemTable. So when flushed, only the latest state per key is written to the SSTable.

---

## SSTables (Sorted String Tables)

An SSTable is an **immutable, sorted** collection of key-value entries stored on disk, named `sstable_1.log`, `sstable_2.log`, etc. Each represents one flushed MemTable.

**Creation, triggered when the WAL threshold is hit:**
1. Collect all MemTable entries
2. Copy into a linear slice
3. Sort lexicographically by key
4. Create a new SSTable file
5. Write sorted entries sequentially
6. Generate a sparse index
7. Sync both files to disk
8. Truncate the WAL
9. Clear the MemTable

Example: MemTable `{dog:40, apple:10, fish:60, banana:20}` → sorted → SSTable file:
```
PUT apple 10
PUT banana 20
PUT dog 40
PUT fish 60
```

**Why immutable?** Inserting a new key (e.g. `cat`) into an already-sorted on-disk file would require rewriting part or all of it. Instead, GoLSM buffers new writes in a fresh MemTable and later flushes *another* sorted SSTable (`sstable_2.log`, `sstable_3.log`, ...). This keeps writes cheap — at the cost of needing compaction later (see below).

---

## Sparse Indexing

Reading an entire SSTable into memory on every GET would defeat the purpose of keeping data on disk. Even with `O(log n)` binary search, just *reading* the file is `O(n)`.

So each SSTable gets a matching sparse index file (`sstable_1.log` + `sstable_1.index`) storing selected keys and their byte offsets — **not every key**, just periodic checkpoints (currently every **10 entries**).

Example SSTable with byte offsets:
```
0    PUT apple 10
13   PUT banana 20
27   PUT cat 30
...
```
Sparse index might store: `apple 0`, `dog 38`, `grape 77`

**Index generation** (while writing the SSTable): for every entry, if its index is divisible by 10, capture the *current* byte offset (before writing the entry, so the index points to the entry's start) and write `key + offset` to the index file.

---

## Read Path

```
GET(key)
  → Search MemTable
       ├── Found → return value
       └── Not found → Search SSTables (newest → oldest)
```

SSTables are searched newest-to-oldest because later SSTables hold more recent data — the first match found is the correct (latest) value.

**Sparse index lookup, per SSTable:**
```
Open sparse index → Load entries → Binary search
  → Find greatest indexed key ≤ target
  → Retrieve byte offset → Seek directly into SSTable
  → Scan forward
```

Example: index has `apple→0, dog→100, grape→200, mango→300`. Query `GET fish` → closest checkpoint ≤ `fish` is `dog→100` → seek to byte 100 → scan forward (`dog, elephant, fish`) → found.

**Early termination:** since SSTables are sorted, a scan can stop as soon as it passes the target alphabetically. If scanning for `fish` and the file hits `dog, elephant, grape` — since `grape > fish`, `fish` cannot appear later, so the scan aborts and moves to the next older SSTable.

---

## Crash Recovery

On startup the MemTable is empty (RAM state is gone). GoLSM rebuilds it by replaying the WAL:

```
Program starts → Open data.log → Read WAL operations
  → Replay PUT / DELETE → Rebuild MemTable
```

The WAL operation counter is also reconstructed during replay (e.g. `data.log` has 900 ops → after restart counter resets to 0 → after replay it's back to 900), ensuring the flush threshold logic stays correct post-crash.

**SSTable numbering must stay unique across restarts.** At startup, GoLSM scans the directory for files matching `sstable_<number>.log`, extracts the numeric part of each (e.g. `sstable_17.log` → strip `sstable_` and `.log` → `17` → int), finds the max, and sets the next SSTable index to `max + 1`. This prevents existing SSTables from being overwritten.

---

## Hash Table Resizing

The MemTable resizes dynamically based on load factor:
```
load_factor = ENTRIES / BUCKET_SIZE
```
When load factor exceeds **0.75**, bucket count doubles (e.g. 100 → 200). Since bucket assignment depends on `hash(key) % BUCKET_SIZE`, every existing entry must be **rehashed and reinserted** into the new table after a resize.

---

## Current Storage Format

**WAL:**
```
PUT <key> <value>     e.g. PUT username keshav
DELETE <key>          e.g. DELETE username
```

**SSTable:** same `PUT <key> <value>` format, entries sorted lexicographically by key.

**Sparse Index:** `<key> <byte_offset>`, e.g.:
```
apple 0
dog 153
grape 321
mango 502
```
Offsets stored as signed 64-bit integers (matching Go's file offset type).

---

## Complexity

| Operation | Complexity | Notes |
|---|---|---|
| MemTable PUT | O(1) avg | hashing + separate chaining |
| MemTable GET | O(1) avg | |
| MemTable DELETE | O(1) avg | bucket deletion may shift slice elements |
| SSTable flush — collect | O(n) | |
| SSTable flush — sort | O(n log n) | dominates overall flush cost |
| SSTable flush — write | O(n) | sequential write |
| Sparse index search | O(log k) | k = number of index entries, after index is loaded |
| SSTable block scan | small | scans a region near the checkpoint (interval B=10) rather than the whole file |

**Multi-SSTable lookup:** worst case, a GET must search several SSTables — as SSTable count grows, read amplification increases. This is the primary motivation for compaction.

---

## Current Limitations

GoLSM is an educational engine, not production-ready:

- **No tombstones yet** — `DELETE` only removes a key from the MemTable. If that key already exists in an older SSTable, the delete doesn't propagate — reads would still find the old value (fix: introduce tombstone markers).
- **No SSTable compaction** — old key versions across multiple SSTables just accumulate, consuming disk space. Reads still return correct (newest) values since SSTables are searched newest-first, but space is wasted.
- **Sparse indexes are re-parsed on every GET** — no caching; each lookup re-opens and re-parses the index file.
- **Text-based SSTable encoding** — human-readable (`PUT apple red`), which requires string parsing on read. A binary format (`[key len][value len][key bytes][value bytes]`) would be faster and more robust.
- **No Bloom filters** — a missing-key lookup may still need to check multiple SSTables before concluding the key doesn't exist.
- **No concurrency control** — single-process, no synchronization for concurrent readers/writers.
- **Flush is not atomic (crash-during-flush edge case)** — under normal single-threaded operation the WAL stays small (it's truncated as soon as the flush threshold, e.g. 1,000 ops, is hit — it should never realistically approach something like 5,000 ops). But if the process crashes *mid-flush* — after `sstable_N.log` / `sstable_N.index` are written and synced, but before the WAL is truncated — the WAL still holds those same operations. On restart, replay re-inserts a batch that's already been persisted in an SSTable, so the same writes end up represented in both places. This is a flush atomicity/recovery problem, parked here for now — the plan is to move on to tombstones first and revisit atomic flush later.

---

## Planned Improvements

1. **Tombstones** — represent deletes as persistent markers (`DELETE apple` → internally `apple → TOMBSTONE`); GET operations stop searching older SSTables once a tombstone is hit.

2. **SSTable Compaction** — merge multiple SSTables into one newer sorted SSTable:
   ```
   SSTable 1 + SSTable 2 + SSTable 3 → Compaction → Merged SSTable
   ```
   Removes obsolete key versions, preserves the newest value, processes tombstones safely, reduces disk usage and read amplification.

3. **In-Memory Sparse Index Metadata** — load all sparse indexes once at startup and keep them in memory (`SSTable N → []IndexEntry`), so GETs binary-search in-memory instead of re-reading index files from disk each time.

4. **Bloom Filters** — each SSTable maintains a Bloom filter:
   ```
   GET key → Bloom Filter
     ├── definitely absent → skip SSTable
     └── possibly present → check sparse index → seek
   ```
   Expected to significantly speed up missing-key lookups.

5. **Binary SSTable Encoding** — replace text-based records with length-prefixed binary records:
   ```
   | Key Length | Value Length | Key Bytes | Value Bytes |
   ```
   Benefits: reduced parsing overhead, smaller storage footprint, support for spaces and arbitrary byte sequences, more predictable/robust decoding.

6. **Range Queries** — since SSTables are sorted, the engine can support range scans, e.g.:
   ```
   SCAN apple mango
   ```
   returning all keys within a sorted range.

7. **Benchmarking** — the engine will be benchmarked across each implementation stage to quantify *why* each component exists, not just measure the final result. Planned metrics:
   - PUT / GET throughput
   - Random read latency
   - Sequential read latency
   - Missing-key latency
   - p50 / p95 / p99 GET latency
   - WAL recovery time
   - SSTable flush time
   - Bytes read per GET
   - Disk usage
   - Read amplification

   Comparisons will be run incrementally to isolate the effect of each optimization:
   ```
   Baseline (WAL + MemTable)
     → + SSTables
     → + Sparse Index
     → + In-Memory Index Metadata
     → + Bloom Filters
     → + Compaction
     → + Binary Encoding
   ```

---

## Project Goals

GoLSM is built to understand storage engines from first principles. The implementation intentionally builds each component (WAL, MemTable, SSTables, sparse indexing, etc.) directly, rather than relying on an existing embedded database library — the point is learning *why* each piece exists, not just having a working KV store.