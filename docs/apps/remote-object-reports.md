# Report the object opened inside an embedded editor

A remote view can declare `objectReport` and request `context.publish` to recognize an existing message from its iframe. The host checks the source window and the HTTPS origin of the declared view. Only the matched object identifier goes into context. Navigation can be silent, so treat this as the last reported object.

```json
{
  "type": "editor.opened",
  "idPath": "record.id",
  "matchPath": "record.kind",
  "matchValue": "document",
  "clear": ["editor.list"]
}
```

Put this block at `views[].objectReport`. The optional match compares one string field. A matching clear message or a frame reload clears the report. A cooperative SDK view can instead publish `{object: "record-123"}` or `{object: null}`.
