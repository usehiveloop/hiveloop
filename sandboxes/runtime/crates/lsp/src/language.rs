/// Map a file extension to its LSP language identifier.
///
/// Based on the VSCode language identifier spec and common LSP server conventions.
pub fn language_id(ext: &str) -> &'static str {
    match ext {
        // JavaScript / TypeScript
        "js" | "mjs" | "cjs" => "javascript",
        "jsx" => "javascriptreact",
        "ts" | "mts" | "cts" => "typescript",
        "tsx" | "mtsx" | "ctsx" => "typescriptreact",

        // Web
        "html" | "htm" => "html",
        "css" => "css",
        "scss" => "scss",
        "sass" => "sass",
        "less" => "less",
        "vue" => "vue",
        "svelte" => "svelte",
        "astro" => "astro",
        "pug" | "jade" => "jade",

        // Systems
        "rs" => "rust",
        "go" => "go",
        "c" | "h" => "c",
        "cpp" | "cc" | "cxx" | "hpp" | "hxx" | "hh" | "c++" | "h++" => "cpp",
        "zig" | "zon" => "zig",

        // JVM
        "java" => "java",
        "kt" | "kts" => "kotlin",
        "scala" | "sc" => "scala",
        "groovy" | "gvy" => "groovy",
        "clj" | "cljs" | "cljc" | "edn" => "clojure",

        // Scripting
        "py" | "pyi" | "pyw" => "python",
        "rb" | "erb" | "gemspec" | "ru" | "rake" => "ruby",
        "php" => "php",
        "lua" => "lua",
        "pl" | "pm" => "perl",
        "r" | "R" => "r",
        "jl" => "julia",
        "ex" | "exs" => "elixir",
        "erl" | "hrl" => "erlang",
        "gleam" => "gleam",

        // Shell
        "sh" | "bash" | "ksh" => "shellscript",
        "zsh" => "shellscript",
        "fish" => "shellscript",
        "ps1" | "psm1" | "psd1" => "powershell",

        // Config / Data
        "json" | "jsonc" => "json",
        "yaml" | "yml" => "yaml",
        "toml" => "toml",
        "xml" | "xsl" | "xsd" => "xml",
        "ini" | "cfg" => "ini",
        "env" => "dotenv",
        "prisma" => "prisma",

        // Markup
        "md" | "markdown" => "markdown",
        "tex" | "latex" | "bib" => "latex",
        "rst" => "restructuredtext",
        "typ" | "typc" => "typst",

        // .NET
        "cs" => "csharp",
        "fs" | "fsx" | "fsi" | "fsscript" => "fsharp",
        "vb" => "vb",
        "cshtml" | "razor" => "razor",

        // Mobile
        "swift" => "swift",
        "dart" => "dart",

        // Functional
        "hs" | "lhs" => "haskell",
        "ml" | "mli" => "ocaml",
        "elm" => "elm",

        // Database
        "sql" => "sql",
        "graphql" | "gql" => "graphql",

        // Infrastructure
        "tf" => "terraform",
        "tfvars" => "terraform-vars",
        "hcl" => "hcl",
        "nix" => "nix",
        "dockerfile" | "Dockerfile" => "dockerfile",

        // Other
        "proto" => "protobuf",
        "vim" => "viml",

        _ => "plaintext",
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_common_languages() {
        assert_eq!(language_id("rs"), "rust");
        assert_eq!(language_id("ts"), "typescript");
        assert_eq!(language_id("tsx"), "typescriptreact");
        assert_eq!(language_id("js"), "javascript");
        assert_eq!(language_id("py"), "python");
        assert_eq!(language_id("go"), "go");
        assert_eq!(language_id("java"), "java");
        assert_eq!(language_id("c"), "c");
        assert_eq!(language_id("cpp"), "cpp");
        assert_eq!(language_id("rb"), "ruby");
        assert_eq!(language_id("html"), "html");
        assert_eq!(language_id("css"), "css");
        assert_eq!(language_id("json"), "json");
        assert_eq!(language_id("yaml"), "yaml");
        assert_eq!(language_id("toml"), "toml");
        assert_eq!(language_id("sql"), "sql");
        assert_eq!(language_id("sh"), "shellscript");
        assert_eq!(language_id("md"), "markdown");
        assert_eq!(language_id("swift"), "swift");
        assert_eq!(language_id("dart"), "dart");
        assert_eq!(language_id("hs"), "haskell");
        assert_eq!(language_id("ex"), "elixir");
        assert_eq!(language_id("kt"), "kotlin");
        assert_eq!(language_id("cs"), "csharp");
        assert_eq!(language_id("php"), "php");
        assert_eq!(language_id("lua"), "lua");
        assert_eq!(language_id("zig"), "zig");
        assert_eq!(language_id("nix"), "nix");
        assert_eq!(language_id("vue"), "vue");
        assert_eq!(language_id("svelte"), "svelte");
    }

    #[test]
    fn test_new_extensions() {
        assert_eq!(language_id("c++"), "cpp");
        assert_eq!(language_id("h++"), "cpp");
        assert_eq!(language_id("zon"), "zig");
        assert_eq!(language_id("astro"), "astro");
        assert_eq!(language_id("edn"), "clojure");
        assert_eq!(language_id("typ"), "typst");
        assert_eq!(language_id("typc"), "typst");
        assert_eq!(language_id("gemspec"), "ruby");
        assert_eq!(language_id("ru"), "ruby");
        assert_eq!(language_id("rake"), "ruby");
        assert_eq!(language_id("hcl"), "hcl");
        assert_eq!(language_id("tfvars"), "terraform-vars");
        assert_eq!(language_id("cshtml"), "razor");
        assert_eq!(language_id("razor"), "razor");
        assert_eq!(language_id("pug"), "jade");
        assert_eq!(language_id("jade"), "jade");
        assert_eq!(language_id("mtsx"), "typescriptreact");
        assert_eq!(language_id("ctsx"), "typescriptreact");
        assert_eq!(language_id("fsscript"), "fsharp");
        assert_eq!(language_id("gleam"), "gleam");
        assert_eq!(language_id("ksh"), "shellscript");
    }

    #[test]
    fn test_unknown_extension() {
        assert_eq!(language_id("xyz"), "plaintext");
        assert_eq!(language_id(""), "plaintext");
        assert_eq!(language_id("unknown"), "plaintext");
    }

    #[test]
    fn test_variant_extensions() {
        // Multiple extensions for same language
        assert_eq!(language_id("mjs"), "javascript");
        assert_eq!(language_id("cjs"), "javascript");
        assert_eq!(language_id("mts"), "typescript");
        assert_eq!(language_id("cts"), "typescript");
        assert_eq!(language_id("cc"), "cpp");
        assert_eq!(language_id("cxx"), "cpp");
        assert_eq!(language_id("hpp"), "cpp");
        assert_eq!(language_id("pyi"), "python");
        assert_eq!(language_id("htm"), "html");
        assert_eq!(language_id("yml"), "yaml");
    }
}
