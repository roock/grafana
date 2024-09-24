INSERT INTO `secure_value` (
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
    'abc',
    'ns', 'name', 'title',
    'salt', 'value',
    'keeper', 'addr',
    1234, 'user:ryan',
    5678, 'user:cameron',
    '{"x":"XXXX"}', '{"a":"AAA", "b", "BBBB"}',
    '["aaa", "bbb", "ccc"]'
  )
;
