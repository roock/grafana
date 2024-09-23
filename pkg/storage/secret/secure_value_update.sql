UPDATE {{ .Ident "secure_value" }} SET 
 "salt"={{ .Arg .Row.Salt }}, "value"={{ .Arg .Row.Value }}, 
 "keeper"={{ .Arg .Row.Keeper }}, "addr"={{ .Arg .Row.Addr }},
 "updated"={{ .Arg .Row.Updated }}, "updated_by"={{ .Arg .Row.UpdatedBy }},
 "annotations"={{ .Arg .Row.Annotations }}, "labels"={{ .Arg .Row.Labels }}, 
 "apis"={{ .Arg .Row.APIs }}
WHERE "uid"={{ .Arg .Row.UID }}
  AND "namespace"={{ .Arg .Row.Namespace }}
  AND "name"={{ .Arg .Row.Name }}
;