UPDATE `secure_value` SET 
 "salt"='salt', "value"='vvv', 
 "keeper"='keeper', "addr"='addr',
 "updated"=5678, "updated_by"='user:cameron',
 "annotations"='{"x":"XXXX"}', "labels"='{"a":"AAA", "b", "BBBB"}', 
 "apis"='["aaa", "bbb", "ccc"]'
WHERE "uid"='uid'
  AND "namespace"='ns'
  AND "name"='name'
;
