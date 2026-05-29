def resolve:
  if type == "array" then
    map(resolve)
  elif type == "object" then
    . as $object
    | reduce ($object | keys_unsorted[]) as $key ({};
        if $key == "files_ref" then
          . + {
            "files": (
              reduce ($object[$key] | keys_unsorted[]) as $file_key ({};
                . + {($file_key): $text_refs[$object[$key][$file_key]]}
              )
            )
          }
        elif ($key | endswith("_json_ref")) then
          . + {($key | sub("_json_ref$"; "")): $json_refs[$object[$key]]}
        elif ($key | endswith("_ref")) then
          . + {($key | sub("_ref$"; "")): $text_refs[$object[$key]]}
        else
          . + {($key): ($object[$key] | resolve)}
        end
      )
  else
    .
  end;

def apply_model_catalog($catalog):
  if type == "array" then
    map(apply_model_catalog($catalog))
  elif type == "object" then
    . as $object
    | reduce ($object | keys_unsorted[]) as $key ({};
        if $key == "model_catalog" or $key == "model_key" then
          .
        else
          . + {($key): ($object[$key] | apply_model_catalog($catalog))}
        end
      )
    | if $object.model_key? then
        ($catalog[$object.model_key] // error("unknown model_key: \($object.model_key)")) as $model
        | .model = $model
        | if .context?.compaction? then
            .context.compaction.summarizer_model = $model
          else
            .
          end
      else
        .
      end
  else
    .
  end;

($template[0] | resolve) as $resolved
| $resolved | apply_model_catalog($resolved.model_catalog)
