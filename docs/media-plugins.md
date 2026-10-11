# Media plugins

Image, audio and video files are annotatable from milestone 9. They are
served by gloss itself, so nothing leaves your machine.

| Format | Extensions | Anchor | How to annotate |
|---|---|---|---|
| Image | png, jpg, jpeg, gif, webp, svg | `region` | Drag a rectangle on the image |
| Audio | mp3, wav, ogg, m4a, flac | `time` | Play, then **Mark start** and **Mark end** |
| Video | mp4, webm, mov, ogv | `region_time` | Pause on a frame, **Draw region**, optionally **Mark end** |

Existing threads show as numbered boxes on an image, as chips under an
audio or video player (click one to seek), and always in the thread
panel. A video box appears only while the playhead is inside the
thread's time range.

## Coordinates

Regions are fractions from 0 to 1 of the displayed image or frame, with
the origin at the top left, so they are independent of zoom and window
size. Times are seconds. An agent creating a thread over MCP passes the
same units; see [mcp-api.md](mcp-api.md) and
[data-model.md](data-model.md#anchor).

## Limits and behaviour

- Files over 25 MB show a "too large to preview" note and cannot be
  annotated.
- SVG is displayed as an image only, never inlined, so its script cannot
  run.
- When the file changes on disk, its region and time threads are
  orphaned: a position in old pixels or audio means nothing in the new
  file. They stay visible in the thread panel and can be reopened.
- Playback depends on your browser's codecs. A file the browser cannot
  decode still shows its player, but you cannot seek it.
- pdf is not supported yet.
