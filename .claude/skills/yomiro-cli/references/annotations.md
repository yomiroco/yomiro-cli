# Annotating training projects from the CLI

A **training project** is what teaches a device what to look for. It has **labels**
(`dog`, `person`, `scratch`, …), **media** (photos, or videos), and **shapes**: the boxes or
polygons drawn on a photo, each tied to one label. Pushing the project sends the boxed
photos to the project's linked devices, which use them as visual examples of each label.

The web app's **Captures → Training** page and the `yomiro annotation` group act on the same
data. You can draw in the browser and fix boxes from a script, or the other way round.

Annotation commands need the `inspection` scope. `yomiro auth login` mints a read-only
key by default, so add write access before you create, change or push anything:

```bash
yomiro auth login --scopes inspection:read,inspection:write,devices:read
```

## Find what you're working on

```bash
yomiro annotation list-projects | jq '.data[] | {id, name}'
yomiro annotation get-project-with-stats <projectId>   # label_count, media_count, annotation_count
yomiro annotation list-labels --project-id <projectId> # label names → ids
yomiro annotation list-media  --project-id <projectId> # media ids, width, height
yomiro annotation get-media-with-shapes <mediaId>      # one photo with every shape on it
```

`get-media-with-shapes` is the quickest way to see what's on one photo: each shape has its
`id`, `label_id`, `shape_type` and `geometry`.

## Coordinates: 0–1000, not pixels

Shape points are `{"x": …, "y": …}` objects on a **0–1000 grid** across the photo's width
and height, whatever the resolution. `(0, 0)` is the top-left corner and `(1000, 1000)` the
bottom-right, on a 1280×720 photo and a 2560×1440 photo alike.

To convert from pixels, use the photo's `width` and `height` from `list-media`:

```
x = round(px / width  * 1000)
y = round(py / height * 1000)
```

A box (`shape_type: "bbox"`) is four corners, clockwise from the top-left:

```json
{
  "points": [
    {"x": 490, "y": 337}, {"x": 737, "y": 337},
    {"x": 737, "y": 765}, {"x": 490, "y": 765}
  ]
}
```

> **Pixel values are the most common mistake.** The API refuses any point outside 0–1000
> with a 400 that says how to convert. Pixel values *under* 1000 can't be caught, though: on
> a 2560×1440 photo, a pixel box at x=400 is accepted and drawn at 40% of the width instead
> of 16%. Always convert, even when the numbers look small. `[x, y]` pairs are accepted
> and stored as `{x, y}` points.
>
> Boxes written before this check existed can still hold pixels or pairs. In the web editor
> they show up as a label tag pinned to the photo's edge with no box, or as nothing at all.
> Use the example below to find and remove them.

## Add, fix and remove boxes

```bash
# Add one box. --skeleton prints every field; the ones below are what a photo needs.
yomiro annotation create-shape --json-body '{
  "annotation_media_id": "<mediaId>",
  "label_id": "<labelId>",
  "shape_type": "bbox",
  "frame_index": 0,
  "is_keyframe": true,
  "geometry": {"points": [{"x":490,"y":337},{"x":737,"y":337},{"x":737,"y":765},{"x":490,"y":765}]}
}'

# Several boxes on one photo in one call
yomiro annotation create-shapes-bulk --json-body @boxes.json   # {"annotation_media_id": …, "shapes": [ …same fields… ]}

# Move or resize a box: send the new geometry (or a new label_id)
yomiro annotation update-shape <shapeId> --json-body '{"geometry": {"points": [ … ]}}'

# Remove a box
yomiro annotation delete-shape <shapeId>
```

On a photo, `frame_index` is always `0`. On a video it is the frame number the box belongs
to. Only boxes with `is_keyframe: true` are sent to devices.

### Example: clear out boxes from before the 0–1000 check

Old boxes stored as `[x, y]` pairs were pixels, even when every value is under 1000, so the
filter removes those as well as anything off the grid:

```bash
yomiro annotation get-media-with-shapes <mediaId> \
  | jq -r '.shapes[]
      | select(any(.geometry.points[]?; type == "array")
               or ([.geometry.points[]? | (.x? // .[0]), (.y? // .[1])] | max > 1000))
      | .id' \
  | xargs -n1 yomiro annotation delete-shape
```

Then redraw those boxes on the 0–1000 grid, or in the web editor.

## Send the project to devices

```bash
yomiro annotation get-links-for-project <projectId>             # which devices / groups it goes to
yomiro annotation quick-link-device-group <projectId> <groupId> # link a device group
yomiro annotation preview-push --project-id <projectId>         # what would be sent
yomiro annotation create-push --json-body '{"project_id": "<projectId>", "device_group_id": "<groupId>"}'
yomiro annotation list-pushes --project-id <projectId>          # status of recent pushes
```

Push again after every change. A device keeps the last push it received until it gets a
new one.
