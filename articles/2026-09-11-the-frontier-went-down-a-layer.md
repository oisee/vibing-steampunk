# VSP IS *STILL* ONLY 5% EXPLORED — And Now a Real SAP GUI Is Drawing Our Pixels

**Or: since April — 209 stars, 538 commits, 17 releases, a rogue server that a live SAP GUI paints fireworks from, and a SAP kernel that ate bytes we compressed ourselves**

---

There is a file in this repository called `articles/2026-04-07-vsp-only-5-percent-explored.md`. It was finished on April 7 and never published. In August I wrote its sequel — *"VSP Is Still Only 5% Explored"* — and didn't publish that one either.

I am not breaking the streak. I am writing the *next* sequel, because something happened after April that neither of the earlier drafts saw coming: the project stopped being a client of SAP and started being a **server** of it. We went a layer *down*. And a real SAP GUI — the actual SAP Logon window, the beige one your whole company has open right now — connected to a few hundred lines of Go and drew fireworks.

Let me back up.

---

## The Scoreboard (Since April)

| Metric | April 7 | Today (Sep 11) | Delta |
|---|:---:|:---:|:---:|
| **GitHub Stars** | 257 | **466** | +209 |
| **Forks** | 58 | **114** | +56 |
| **Commits** | 455 | **993** | +538 |
| **Releases** | v2.38.1 | **v2.56.0** (72 published) | +17 |
| **Tests** | 821 | **1,362 functions** across 28 packages | +541 |
| **Sibling repos** | 1 | **4**, with a shared knowledge base | +3 |

The last row is the one that changed the shape of everything. In April this was one Go binary that spoke ADT over HTTPS. Today it is a **family**: `vsp` (ADT + RFC + the debugger), `open-rfc-go` (the NI/RFC wire), `open-diag-go-pro` (the *GUI* wire), and `sap-kb` — a cross-repo index that maps the four against each other so a session in one knows what the others already learned. April's "5% explored" was a claim about breadth. This period the *denominator moved down a floor.*

---

## The Big Shape Change: We Went Below ADT

Everything before April lived at the ADT layer — the polite, documented, HTTPS REST surface SAP gives to tools. This period we dropped beneath it, to **DIAG**: the ancient binary protocol the SAP GUI itself speaks to the dispatcher on port 32xx. Nobody documents it. So we reverse-engineered enough of it to build a **rogue server** — and then made it do things the real one never would.

### 1. A real SAP GUI, drawing frames from Go

`open-diag-go-pro` is a pure-Go DIAG server with **no SAP system behind it**. A genuine SAP GUI connects, sends its logon, and from that moment every pixel it draws comes from bytes we wrote in Go — dynpro screens, classic lists, icons, status messages, pushed at our own cadence.

Once you can push a frame, you can push a *different* frame 60 milliseconds later. So the demo engine happened:

- **fireworks** — rockets climbing as labels, bursting into rings of sparks that fall under gravity, all placed cell by cell;
- **a double helix** twisting in place, two sine strands with rungs between them;
- **an equalizer** whose bars are pushbuttons — the button's *Height* field *is* the graphics, not a drawn glyph;
- **a starfield**, a Lissajous **snake** with a fading trail, a **Matrix** rain;
- and an **LED display** in the classic-list colour channel, running plasma and rings with a letter-density ramp for smooth halftone.

A real SAP GUI. Rendering a plasma effect. This is not what SAP GUI is for, and that is the entire point.

### 2. The family's only *encoder* — and a live kernel ate it

Here is the technically delicious part. `vsp` and `open-rfc-go` only ever *decode* SAP's compression — LZH (which is DEFLATE wearing an eight-byte SAP header and a two-bit prefix) and LZC (compress(1)'s LZW). Decoding is enough to read a data cluster or a DIAG frame. But to *push* a compressed frame you need to **write** the format, and nobody in the family ever had.

So `open-diag-go-pro/pkg/alv` grew the family's first **SAP-LZH writer** — and immediately hit a wall that took a full day to understand:

> A real SAP kernel does not run a general DEFLATE inflater. It reads the stream at a **two-bit offset** with a continuous bit reader, and DEFLATE *stored blocks* — the one construct that must be byte-aligned — desync it. Feed it a stored block and the GUI **hangs, forever, waiting for more.**

Our own decoder is lenient (it shifts the whole buffer back into alignment first, then hands it to `compress/flate`), so a stream that round-trips through *us* can still hang the *kernel*. That distinction cost real hours. The fix is a bit-level DEFLATE assembler that emits **Huffman-only, exactly the original length, properly terminated** — it un-finalises `flate`'s last block and pads with empty Huffman blocks to the target byte.

And then the payoff. On a **live A4H trial**, a client body we compressed with that writer was sent to the dispatcher — and **the kernel decompressed it and processed it** (915 bytes on the wire, 685 out). `compress=0` hung; our `compress=2` did not. The family's first encoder, validated not against our own decoder but against **SAP's actual runtime.** The bytes were ours. It ate them.

### 3. The ALV wall, and a law we can now state

We can decode an ALV grid's data blob — three nested levels of SAP-LZH, RFC-row-chunked, down to a per-cell colour byte — and re-synthesize one from scratch. What we *cannot* do is recolour a replayed grid and have the live GUI render it. For a while we blamed our compression. We were wrong, and a **real ABAP short dump** proved it.

Drive any frontend control off a *replayed* control channel and the live server dumps: `MESSAGE_TYPE_X` in `SAPLOLEA` — the OLE-automation controller — at `MESSAGE X373 WITH '-1'`. Root cause, confirmed from the dump's own system fields and selected variables: the replayed `RFC_TR.01` carries the **control GUID and result-variable pointers of the session that was captured**, and in a new session those handles resolve to nothing (`RESULT_VARS_WA … POINTER = 0x0`). The compression was never the problem — the kernel proved it accepts our bytes. The block is one layer up, in the session-bound Control Framework payload.

Which gives the sharpest law this period taught:

> **The control channel cannot be replayed — it must be *participated in.*** Create the control in *your* session, read back the GUID the kernel assigns you, and thread *that* through every automation call. A captured control batch is only ever valid inside its own session.

### 4. The RCA loop closed on itself

And here is where the family ate its own tail, beautifully. Those DIAG experiments produced a real short dump on a live system — and `vsp`, the *other* repo, is the one with the dump tooling. So `vsp dumps --explain` met the very `SAPLOLEA` dump the GUI work had just created.

Except the dump *model* was shallow — `{id, when, user, error, program, message}` plus a fail line. The "why" — the message coordinates, the failing statement, the variable that held the null pointer — was in ST22's own document but not in the model, so you read it by hand. This week that got fixed: `DumpDetail` now parses the **whole formatted document** it already fetches — the `Contents of system fields` table (`SY-MSGID`, `SY-MSGNO`, `SY-MSGV1..4`, `SY-SUBRC`, `SY-PFKEY` …), the `Source Code Extract` with the failing line flagged, and the `Selected Variables` chapter per stack frame. `--explain` now opens with the answer:

```
message SY 373 (type X)  with -1
SUBRC=4  FDPOS=40  PFKEY=SESSION_ADMIN  TITLE=SAP Easy Access
source:
    530  else.
  > 531  MESSAGE X373 WITH '-2'.     "=================> Error
    532  endif.
```

No extra round trips — it was all in the one document. Validated against the live A4H dump the DIAG server produced. The instrument that finds root causes met a root cause its own sibling had manufactured, and explained it. That is the family working as one thing.

---

## Also, Since April (the ADT layer kept moving too)

The stuff the August draft covered, still true and still the backbone:

- **ADT debugging with nothing installed** — the "REST breakpoints 403 on newer SAP" myth was our own stateless client all along; and **AMDP debugging** turned out to be native ADT the whole time, in the discovery document as template links.
- **Classic RFC in pure Go, client *and server*, no NetWeaver SDK, no cgo** — a live system ran six parametrised calls against a Go endpoint, every one `rc=0`, all three SM59 buttons green.
- **Two-directional context** — a dependency trimmed to the methods actually called (whole contexts 27% smaller), and *who calls this* (`Called from 73 places in 45 packages`), which no source can supply.
- **The tool that checks itself** — `vsp sweep` walks the advertised surface with an **oracle** for every probe that could return a false-empty, and reports `dead` versus `no results`. Swept clean on 7.58 / 7.57 / 7.50: **42 probes, 0 defects standing.** Because a second system is needed not for coverage but for *disagreement* — one system cannot refute itself.
- **Cluster tables decoded** — BALDAT, INDX, STXL over plain ADT, SAP's LZH/LZC in Go. (The same decoder whose *inverse* just fed a live kernel.)

---

## The Honest Assessment, Updated

**What works, and is now checked on live systems**
- A rogue DIAG server a real SAP GUI draws from, frame by frame
- The family's SAP-LZH **writer**, accepted by a live A4H kernel (C→S)
- The full ALV blob codec — decode and synthesize — down to the per-cell colour
- ADT + AMDP debugging with nothing installed; classic RFC both directions
- Dump post-mortem that now shows the *why*, not just the stack

**What is still hard, or simply a law of the terrain**
- **Live ALV recolour, S→C.** Not our compression — the kernel proved that. The Control Framework payload is session-bound, so a replayed grid carries a stale handle. Parked with a law attached: *participate, don't replay.*
- SNC. Plain-DIAG logon to the trial's own endpoint is refused at the security layer — not a bug, just the terrain. (ADT-over-HTTPS sails past it, which is how the dumps got read.)
- 14 open PRs.

**What surprised me**
- That the coolest artefact of the whole period was **a SAP kernel decompressing bytes a Go program wrote** — the inverse of a decoder we'd had since December, and it only took believing the hang was *stored blocks at a two-bit offset.*
- That a real `SAPLOLEA` short dump would end up being the cleanest possible *proof* of a reverse-engineering conclusion, read by the very tool the experiment fed.

---

## Why *Still* 5%

Because the denominator moved *down.*

In April, "5% explored" meant *there are 147 tools and most people use eight* — a claim about breadth at the ADT layer. In August it meant *most of the map was never unbuilt, it was unchecked* — a claim about verification. Today it means something new: **there is a whole protocol floor below the one we'd been standing on**, and we just cut the first window into it. The GUI wire. The compression the kernel actually accepts. The session-bound control channel and its one law.

The verified piece is still small. But the map didn't just get wider this period — it got *taller*, and the new floor has a real SAP GUI standing on it, drawing our pixels.

Which remains a much better problem than the one April had, because it still has a method attached.

---

**GitHub**: [oisee/vibing-steampunk](https://github.com/oisee/vibing-steampunk) · **v2.56.0** · 466 stars · 72 releases · plus `open-rfc-go`, `open-diag-go-pro`, and a shared `sap-kb`

*Previously: "Agentic ABAP: Why I Built a Bridge for Claude Code" (Dec 2025) · "…at 100 Stars" (Feb 2026) · "VSP Is Only 5% Explored" (Apr 2026) · "…Still Only 5%" (Aug 2026) — all written, none published. This one keeps the tradition.*

#ABAP #SAP #MCP #ClaudeCode #GoLang #OpenSource #AI #S4HANA #DIAG #SAPGUI #ReverseEngineering #RFC #ALV #VSP
