# pybr
CLI that scans a Python codebase and reports where each method is **defined** and **used**.
It wraps [ripgrep](https://github.com/BurntSushi/ripgrep) for speed and prints to multiple formats (console, json, vimgrep).

## Why
I needed a fast way to spot **unused Python methods**. I used to manually search each method using
either pyright or ripgrep, then I look for any other tools that could help.
Two of them are Vulture or MyPy, which are great, but:
- Vulture can be conservative or miss dynamic cases.
- MyPy focuses on static typing, not grep-like cross references.

`pybr` is a pragmatic, grep-based approach: find `def` lines, then grep usages across the tree.

## Install
- Requires **ripgrep (`rg`)** in your PATH.
- Go ≥ 1.25.2

## Examples
Let's search on `directory-to-search ` unused methods (`-max-usages 1`):

```bash
py-broom main ❯ ./pybr --dir /home/samuel/Documentos/med-seg-tfm/src --skip-private --max-usages 1 --format json | jq
[
  {
    "method": {
      "name": "visualize",
      "filename": "/home/samuel/Documentos/med-seg-tfm/src/steps/visualization/visualize.py",
      "line_number": 8
    },
    "usages": [
      {
        "location": "/home/samuel/Documentos/med-seg-tfm/src/steps/visualization/visualize.py:8:5",
        "call_type": "definition",
        "context": "def visualize(config: VisualizeConfig) -> None:"
      }
    ],
    "usages_by_type": {
      "definition": 1
    },
    "total_usages": 1
   }
  }
]
```

Here we can see that `output_default_config` and `normalize_to_uint8` are defined but not used anywhere.

Another useful feature is that we can format the output to quickly jump using [QuickFix](https://neovim.io/doc/user/quickfix.html):
```bash
pybr directory-to-search -max-usages 1 -format vimgrep

/home/samuel/Documentos/med-seg-tfm/src/cli/cli_utils.py:173:5:def output_default_config(to_stdout: bool, filename: str | None) -> None:
/home/samuel/Documentos/med-seg-tfm/src/dashboard.py:63:5:def normalize_to_uint8(slice2d: np.ndarray) -> np.ndarray:
```


