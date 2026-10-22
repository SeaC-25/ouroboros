// Package desktopcontract ports gui/scripts/check-desktop-contract.mjs.
//
// Exit codes: 0 = pass, 1 = contract failure, 2 = environment/IO error
// (e.g. the node executable needed by the dynamic module contracts cannot
// be spawned).
//
// Limitations, matching the original script's semantics:
//   - Substring (`includes`) assertions are naive: a needle that appears only
//     inside a `//` line comment or a string literal still counts as present,
//     because the original mjs asserts raw text and intentionally so.
//   - The dynamic contracts (memory relaunch args, backend exit
//     normalization, runtime PID handoff, user installer) are delegated to
//     `node` driver scripts that import the exact same JS modules and run the
//     exact same assertions as the mjs, so behaviour stays 1:1.
package desktopcontract
