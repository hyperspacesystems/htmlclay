'use strict';

// Stands in for the tailwindcss package inside the packed program (build.mjs).
// HTML Clay compiles every stylesheet in Go with the Tailwind program it carries
// (internal/tailwind), and Go answers /tailwindcss/ itself, so this program uses
// only tailwind-hyperclay's link helpers and never compiles.
exports.compile = async () => {
  throw new Error('the live sync program does not compile Tailwind; HTML Clay compiles it in Go');
};
