//! Pure-Rust command router for the Bash tool's rtk integration.
//!
//! Decides whether each segment of a shell command should be prepended with
//! `rtk ` so that running the command under `sh -c` routes it through rtk's
//! filter pipeline. The whole design is one pass, one allowlist:
//!
//! 1. Hard-coded allowlist of commands rtk can filter — rtk's Clap
//!    subcommands, rtk's built-in TOML filters, and whatever our embedded
//!    filters.toml covers (composer / php artisan / vendor/bin/*).
//! 2. Split the input command on top-level `&&`, `||`, `;`, `|`, respecting
//!    `"` / `'` quoting and `\` escapes. Heredocs short-circuit to a single
//!    segment (losing filtering on those is strictly safer than corrupting
//!    the command).
//! 3. For each segment: skip env-assignment prefixes (`VAR=value ...`) and
//!    `sudo`, take the next bare word, look it up in the allowlist; if
//!    found, insert `rtk ` at that position.
//! 4. Rejoin segments with their original operators — whitespace and
//!    quoting are preserved exactly.
//!
//! Why not use `rtk rewrite` as the gate? Its rewrite registry is hardcoded
//! in rtk's binary (`src/discover/rules.rs`) and does not know about Laravel
//! commands. Calling `rtk rewrite "php artisan migrate"` returns exit 1
//! (passthrough), so bridge would run the command unfiltered even though we
//! have a filter for it. Doing the dispatch decision here closes the gap
//! without forking rtk.

/// Commands that rtk can filter. First-word match; compound commands are
/// split and each segment is checked independently.
///
/// This is a strict superset of rtk's own `rtk rewrite` registry — we cover
/// rtk's Clap subcommands, the TOML filters rtk ships built-in, and the
/// Laravel/PHP families we added in our filters.toml. Kept alphabetical
/// within each section for discoverability.
const RTK_COMMANDS: &[&str] = &[
    // --- rtk Clap subcommands with specialized Rust filter code ---
    "aws",
    "cargo",
    "curl",
    "diff",
    "docker",
    "dotnet",
    "find",
    "format",
    "gh",
    "git",
    "go",
    "golangci-lint",
    "grep",
    "gt",
    "jest",
    "kubectl",
    "lint",
    "log",
    "ls",
    "mypy",
    "next",
    "npm",
    "npx",
    "pip",
    "playwright",
    "pnpm",
    "prettier",
    "prisma",
    "psql",
    "pytest",
    "rake",
    "read",
    "rspec",
    "rubocop",
    "ruff",
    "tree",
    "tsc",
    "vitest",
    "wc",
    "wget",
    // --- Built-in TOML filters shipped inside rtk (rtk src/filters/*.toml) ---
    "basedpyright",
    "biome",
    "fail2ban-client",
    "gcloud",
    "iptables",
    "jq",
    "liquibase",
    "mix-format",
    "mvn-build",
    "poetry-install",
    "rsync",
    "shellcheck",
    "sops",
    "spring-boot",
    "stat",
    "systemctl-status",
    "tofu-init",
    "xcodebuild",
    // --- Laravel / PHP ecosystem (our embedded filters.toml) ---
    "./artisan",
    "artisan",
    "composer",
    "larastan",
    "pest",
    "php",
    "phpstan",
    "phpunit",
    "pint",
    "vendor/bin/larastan",
    "vendor/bin/pest",
    "vendor/bin/phpstan",
    "vendor/bin/phpunit",
    "vendor/bin/pint",
    "vendor/bin/sail",
];

/// Rewrite a shell command, prepending `rtk ` to every segment whose first
/// bare word is in RTK_COMMANDS.
///
/// Idempotent: already-rtk commands are returned unchanged. Safe for
/// commands containing quotes, escapes, env prefixes, `sudo`. Heredocs
/// are passed through verbatim.
pub fn rewrite(cmd: &str) -> String {
    if cmd.is_empty() {
        return String::new();
    }
    if contains_heredoc(cmd) {
        return cmd.to_string();
    }

    let pieces = split_preserving_ops(cmd);
    let mut out = String::with_capacity(cmd.len() + 8);
    for piece in pieces {
        match piece {
            Piece::Segment(s) => out.push_str(&rewrite_segment(&s)),
            Piece::Op(op) => out.push_str(&op),
        }
    }
    out
}

enum Piece {
    Segment(String),
    Op(String),
}

fn contains_heredoc(cmd: &str) -> bool {
    // Any `<<` triggers the escape hatch — quoted `<<` inside a string is
    // rare enough that false-positives here cost only a missed rewrite, not
    // a corrupted command.
    cmd.contains("<<")
}

/// Tokenize on top-level `&&`, `||`, `;`, `|` operators, respecting `'`
/// and `"` quoting and `\`-escapes outside of quotes.
fn split_preserving_ops(cmd: &str) -> Vec<Piece> {
    let mut pieces = Vec::new();
    let bytes = cmd.as_bytes();
    let mut buf = String::new();
    let mut i = 0;
    let mut quote: Option<u8> = None;

    while i < bytes.len() {
        let c = bytes[i];
        if let Some(q) = quote {
            // Inside a quoted string: operators don't count, but \" escapes
            // still matter inside double quotes.
            buf.push(c as char);
            if q == b'"' && c == b'\\' && i + 1 < bytes.len() {
                buf.push(bytes[i + 1] as char);
                i += 2;
                continue;
            }
            if c == q {
                quote = None;
            }
            i += 1;
            continue;
        }
        match c {
            b'\'' | b'"' => {
                quote = Some(c);
                buf.push(c as char);
                i += 1;
            }
            b'\\' if i + 1 < bytes.len() => {
                buf.push(c as char);
                buf.push(bytes[i + 1] as char);
                i += 2;
            }
            b'&' if i + 1 < bytes.len() && bytes[i + 1] == b'&' => {
                flush_segment(&mut pieces, &mut buf);
                pieces.push(Piece::Op("&&".to_string()));
                i += 2;
            }
            b'|' if i + 1 < bytes.len() && bytes[i + 1] == b'|' => {
                flush_segment(&mut pieces, &mut buf);
                pieces.push(Piece::Op("||".to_string()));
                i += 2;
            }
            b'|' => {
                flush_segment(&mut pieces, &mut buf);
                pieces.push(Piece::Op("|".to_string()));
                i += 1;
            }
            b';' => {
                flush_segment(&mut pieces, &mut buf);
                pieces.push(Piece::Op(";".to_string()));
                i += 1;
            }
            _ => {
                buf.push(c as char);
                i += 1;
            }
        }
    }
    if !buf.is_empty() {
        pieces.push(Piece::Segment(buf));
    }
    pieces
}

fn flush_segment(pieces: &mut Vec<Piece>, buf: &mut String) {
    if !buf.is_empty() {
        pieces.push(Piece::Segment(std::mem::take(buf)));
    }
}

/// Rewrite a single segment: preserves leading whitespace, skips env/sudo,
/// inserts `rtk ` before the bare command if it is routable.
fn rewrite_segment(segment: &str) -> String {
    let leading_ws_len = segment
        .bytes()
        .take_while(|b| b.is_ascii_whitespace())
        .count();
    let leading_ws = &segment[..leading_ws_len];
    let body = &segment[leading_ws_len..];
    if body.is_empty() {
        return segment.to_string();
    }

    let Some((prefix_len, first_word)) = extract_first_word_after_prefix(body) else {
        return segment.to_string();
    };

    if first_word == "rtk" || !is_routable(first_word) {
        return segment.to_string();
    }

    let mut out = String::with_capacity(segment.len() + 4);
    out.push_str(leading_ws);
    out.push_str(&body[..prefix_len]);
    out.push_str("rtk ");
    out.push_str(&body[prefix_len..]);
    out
}

/// Skip leading env-assignment prefixes (`FOO=bar BAR=baz ...`) and `sudo`.
/// Returns `(byte_offset_in_body, first_bare_command_word)` — the offset is
/// where to splice `rtk ` into the body.
fn extract_first_word_after_prefix(body: &str) -> Option<(usize, &str)> {
    let mut pos = 0;
    let bytes = body.as_bytes();
    loop {
        while pos < bytes.len() && bytes[pos].is_ascii_whitespace() {
            pos += 1;
        }
        if pos >= bytes.len() {
            return None;
        }
        let token_start = pos;
        while pos < bytes.len() && !bytes[pos].is_ascii_whitespace() {
            pos += 1;
        }
        let token = &body[token_start..pos];
        if token == "sudo" || is_env_assignment(token) {
            continue;
        }
        return Some((token_start, token));
    }
}

fn is_env_assignment(tok: &str) -> bool {
    let Some(eq) = tok.find('=') else {
        return false;
    };
    if eq == 0 {
        return false;
    }
    tok[..eq].chars().enumerate().all(|(i, c)| {
        (i == 0 && (c.is_ascii_uppercase() || c == '_'))
            || (i > 0 && (c.is_ascii_uppercase() || c.is_ascii_digit() || c == '_'))
    })
}

fn is_routable(cmd: &str) -> bool {
    RTK_COMMANDS.contains(&cmd)
}

#[cfg(test)]
mod tests {
    use super::*;

    // --- simple routable commands ---

    #[test]
    fn prepends_for_composer() {
        assert_eq!(rewrite("composer install"), "rtk composer install");
        assert_eq!(rewrite("composer update"), "rtk composer update");
        assert_eq!(
            rewrite("composer require spatie/laravel-permission"),
            "rtk composer require spatie/laravel-permission"
        );
    }

    #[test]
    fn prepends_for_composer_create_project() {
        assert_eq!(
            rewrite("composer create-project laravel/laravel app"),
            "rtk composer create-project laravel/laravel app"
        );
    }

    #[test]
    fn prepends_for_php_artisan() {
        assert_eq!(rewrite("php artisan migrate"), "rtk php artisan migrate");
        assert_eq!(
            rewrite("php artisan install:api"),
            "rtk php artisan install:api"
        );
        assert_eq!(
            rewrite("php artisan vendor:publish"),
            "rtk php artisan vendor:publish"
        );
        assert_eq!(
            rewrite("php artisan make:model Post -mfsc"),
            "rtk php artisan make:model Post -mfsc"
        );
    }

    #[test]
    fn prepends_for_vendor_bin_tools() {
        assert_eq!(rewrite("vendor/bin/pest"), "rtk vendor/bin/pest");
        assert_eq!(
            rewrite("vendor/bin/phpunit --filter X"),
            "rtk vendor/bin/phpunit --filter X"
        );
        assert_eq!(
            rewrite("vendor/bin/pint --test"),
            "rtk vendor/bin/pint --test"
        );
        assert_eq!(
            rewrite("vendor/bin/phpstan analyse --level 9"),
            "rtk vendor/bin/phpstan analyse --level 9"
        );
        assert_eq!(
            rewrite("vendor/bin/sail up -d"),
            "rtk vendor/bin/sail up -d"
        );
    }

    #[test]
    fn prepends_for_built_in_git_cargo_npm() {
        assert_eq!(rewrite("git log --oneline -5"), "rtk git log --oneline -5");
        assert_eq!(rewrite("cargo test"), "rtk cargo test");
        assert_eq!(rewrite("npm install"), "rtk npm install");
        assert_eq!(rewrite("pnpm run build"), "rtk pnpm run build");
        assert_eq!(rewrite("pytest -xvs"), "rtk pytest -xvs");
    }

    // --- commands NOT in the allowlist ---

    #[test]
    fn leaves_unknown_commands_alone() {
        assert_eq!(rewrite("echo hello"), "echo hello");
        assert_eq!(rewrite("my-custom-tool --flag"), "my-custom-tool --flag");
        assert_eq!(rewrite("cat foo.txt"), "cat foo.txt");
    }

    // --- compound commands ---

    #[test]
    fn handles_and_compound() {
        assert_eq!(
            rewrite("cd app && php artisan migrate"),
            "cd app && rtk php artisan migrate"
        );
        assert_eq!(
            rewrite("composer install && php artisan migrate"),
            "rtk composer install && rtk php artisan migrate"
        );
    }

    #[test]
    fn handles_or_compound() {
        assert_eq!(
            rewrite("composer install || echo failed"),
            "rtk composer install || echo failed"
        );
    }

    #[test]
    fn handles_semicolon_compound() {
        assert_eq!(
            rewrite("php artisan migrate; php artisan db:seed"),
            "rtk php artisan migrate; rtk php artisan db:seed"
        );
    }

    #[test]
    fn handles_pipe() {
        assert_eq!(rewrite("git log | grep fix"), "rtk git log | rtk grep fix");
        assert_eq!(
            rewrite("php artisan list | grep make:"),
            "rtk php artisan list | rtk grep make:"
        );
    }

    #[test]
    fn cd_is_not_prepended() {
        assert_eq!(rewrite("cd foo"), "cd foo");
        assert_eq!(rewrite("cd foo && ls -la"), "cd foo && rtk ls -la");
        assert_eq!(
            rewrite("cd /tmp && cd app && php artisan migrate"),
            "cd /tmp && cd app && rtk php artisan migrate"
        );
    }

    // --- env prefixes and sudo ---

    #[test]
    fn env_prefix_before_routable() {
        assert_eq!(
            rewrite("APP_ENV=testing php artisan migrate"),
            "APP_ENV=testing rtk php artisan migrate"
        );
        assert_eq!(
            rewrite("FOO=a BAR=b composer install"),
            "FOO=a BAR=b rtk composer install"
        );
    }

    #[test]
    fn sudo_before_routable() {
        assert_eq!(
            rewrite("sudo composer install"),
            "sudo rtk composer install"
        );
    }

    #[test]
    fn env_and_sudo_before_routable() {
        assert_eq!(
            rewrite("APP_ENV=testing sudo composer install"),
            "APP_ENV=testing sudo rtk composer install"
        );
    }

    // --- idempotency ---

    #[test]
    fn already_rtk_is_unchanged() {
        assert_eq!(rewrite("rtk composer install"), "rtk composer install");
        assert_eq!(
            rewrite("rtk git log && rtk cargo test"),
            "rtk git log && rtk cargo test"
        );
    }

    // --- quoting: operators inside quotes don't split ---

    #[test]
    fn and_inside_double_quotes_is_not_a_splitter() {
        assert_eq!(
            rewrite("echo \"a && b\" && git status"),
            "echo \"a && b\" && rtk git status"
        );
    }

    #[test]
    fn pipe_inside_single_quotes_is_not_a_splitter() {
        assert_eq!(
            rewrite("echo 'foo | bar' | grep foo"),
            "echo 'foo | bar' | rtk grep foo"
        );
    }

    // --- heredocs: bail, don't mess with the command ---

    #[test]
    fn heredoc_is_untouched() {
        let cmd = "cat <<EOF\nhello && php artisan migrate\nEOF";
        assert_eq!(rewrite(cmd), cmd);
    }

    // --- edge cases ---

    #[test]
    fn empty_string() {
        assert_eq!(rewrite(""), "");
    }

    #[test]
    fn whitespace_only() {
        assert_eq!(rewrite("   "), "   ");
    }

    #[test]
    fn leading_whitespace_preserved() {
        assert_eq!(
            rewrite("  php artisan migrate"),
            "  rtk php artisan migrate"
        );
    }

    // --- env assignment detection ---

    #[test]
    fn env_lowercase_var_not_treated_as_env() {
        // `foo=bar` is not a valid POSIX env assignment prefix — treat the
        // token as the command itself (and so not routable).
        assert_eq!(
            rewrite("foo=bar composer install"),
            "foo=bar composer install"
        );
    }

    // --- regression: bench commands the reviewer flagged ---

    #[test]
    fn bench_commands_all_get_routed() {
        // Every command the bench reported as passing through unfiltered now
        // gets rtk-prepended, so the existing TOML filter actually fires.
        assert_eq!(
            rewrite("composer create-project laravel/laravel app \"^13.0\""),
            "rtk composer create-project laravel/laravel app \"^13.0\""
        );
        assert_eq!(rewrite("php artisan migrate"), "rtk php artisan migrate");
        assert_eq!(
            rewrite("php artisan install:api"),
            "rtk php artisan install:api"
        );
        assert_eq!(
            rewrite("php artisan vendor:publish"),
            "rtk php artisan vendor:publish"
        );
        assert_eq!(
            rewrite("composer require spatie/laravel-permission"),
            "rtk composer require spatie/laravel-permission"
        );
    }
}
