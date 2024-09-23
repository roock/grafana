INSERT INTO {{ .Ident "secure_value" }} (
    "uid", 
    "namespace", "name", "title",
    "salt", "value",
    "keeper", "addr",
    "created", "created_by",
    "updated", "updated_by",
    "annotations", "labels", 
    "apis"
  )
  VALUES (
    {{ .Arg .Row.UID }},
    {{ .Arg .Row.Namespace }}, {{ .Arg .Row.Name }}, {{ .Arg .Row.Title }},
    {{ .Arg .Row.Salt }}, {{ .Arg .Row.Value }},
    {{ .Arg .Row.Keeper }}, {{ .Arg .Row.Addr }},
    {{ .Arg .Row.Created }}, {{ .Arg .Row.CreatedBy }},
    {{ .Arg .Row.Updated }}, {{ .Arg .Row.UpdatedBy }},
    {{ .Arg .Row.Annotations }}, {{ .Arg .Row.Labels }},
    {{ .Arg .Row.APIs }}
  )
;
