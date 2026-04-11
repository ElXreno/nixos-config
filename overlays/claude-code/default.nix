{ inputs, ... }:
let
  mkClaude =
    prev:
    inputs.claude-code.packages.${prev.stdenv.hostPlatform.system}.claude-code-node.override {
      nodeBinName = "claude";
    };

  # https://gist.github.com/roman01la/483d1db15043018096ac3babf5688881
  patches = [
    # PATCH 1-3: https://gist.github.com/roman01la/483d1db15043018096ac3babf5688881?permalink_comment_id=6090443#gistcomment-6090443

    # PATCH 4: Anti-gold-plating — allow necessary related work
    {
      old = ''Don't add features, refactor code, or make "improvements" beyond what was asked. A bug fix doesn't need surrounding code cleaned up. A simple feature doesn't need extra configurability. Don't add docstrings, comments, or type annotations to code you didn't change. Only add comments where the logic isn't self-evident.'';
      new = "Don't add unrelated features or speculative improvements. However, if adjacent code is broken, fragile, or directly contributes to the problem being solved, fix it as part of the task. A bug fix should address related issues discovered during investigation. Don't add docstrings, comments, or type annotations to code you didn't change. Only add comments where the logic isn't self-evident.";
    }

    # PATCH 5: Error handling — stop telling the model to skip it
    {
      old = "Don't add error handling, fallbacks, or validation for scenarios that can't happen. Trust internal code and framework guarantees. Only validate at system boundaries (user input, external APIs). Don't use feature flags or backwards-compatibility shims when you can just change the code.";
      new = "Add error handling and validation at real boundaries where failures can realistically occur (user input, external APIs, I/O, network). Trust internal code and framework guarantees for truly internal paths. Don't use feature flags or backwards-compatibility shims when you can just change the code.";
    }

    # PATCH 6: Remove "three lines better than abstraction" rule
    {
      old = "Three similar lines of code is better than a premature abstraction.";
      new = "Use judgment about when to extract shared logic. Avoid premature abstractions for hypothetical reuse, but do extract when duplication causes real maintenance risk.";
    }

    # PATCH 7: Subagent addendum — strengthen completeness
    {
      old = "Complete the task fully—don't gold-plate, but don't leave it half-done.";
      new = "Complete the task fully and thoroughly. Do the work that a careful senior developer would do, including edge cases and fixing obviously related issues you discover. Don't add purely cosmetic or speculative improvements unrelated to the task.";
    }

    # PATCH 8: Explore agent — remove speed-over-thoroughness bias
    {
      old = builtins.concatStringsSep "\n" [
        "NOTE: You are meant to be a fast agent that returns output as quickly as possible. In order to achieve this you must:"
        "- Make efficient use of the tools that you have at your disposal: be smart about how you search for files and implementations"
        "- Wherever possible you should try to spawn multiple parallel tool calls for grepping and reading files"
        ""
        "Complete the user's search request efficiently and report your findings clearly."
      ];
      new = builtins.concatStringsSep "\n" [
        "NOTE: Be thorough in your exploration. Use efficient search strategies but do not sacrifice completeness for speed:"
        "- Make efficient use of the tools that you have at your disposal: be smart about how you search for files and implementations"
        "- Wherever possible you should try to spawn multiple parallel tool calls for grepping and reading files"
        ''- When the caller requests "very thorough" exploration, exhaust all reasonable search strategies before reporting''
        ""
        "Complete the user's search request thoroughly and report your findings clearly."
      ];
    }

    # PATCH 9: Tone — remove redundant "short and concise"
    {
      old = "Your responses should be short and concise.";
      new = "Your responses should be clear and appropriately detailed for the complexity of the task.";
    }

    # PATCH 10: Subagent output — stop suppressing code context
    {
      old = "Include code snippets only when the exact text is load-bearing (e.g., a bug you found, a function signature the caller asked for) — do not recap code you merely read.";
      new = "Include code snippets when they provide useful context (e.g., bugs found, function signatures, relevant patterns, code that informs the decision). Summarize rather than quoting large blocks verbatim.";
    }

    # PATCH 11: Scope matching — allow necessary adjacent work
    {
      old = "Match the scope of your actions to what was actually requested.";
      new = "Match the scope of your actions to what was actually requested, but do address closely related issues you discover during the work when fixing them is clearly the right thing to do.";
    }
  ];

in
_final: prev: {
  claude-code =
    let
      mkReplaceArg =
        p: "--replace-fail ${prev.lib.escapeShellArg p.old} ${prev.lib.escapeShellArg p.new}";
    in
    (mkClaude prev).overrideAttrs (old: {
      postInstall = ''
        ${old.postInstall or ""}

        substituteInPlace $out/lib/node_modules/@anthropic-ai/claude-code/cli.js \
            ${prev.lib.concatStringsSep " " (map mkReplaceArg patches)}
      '';
    });
}
