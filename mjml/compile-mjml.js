#!/usr/bin/env node

const mjml = require('mjml');

if (process.argv.length !== 3) {
  console.error('Usage: node compile-mjml.js <mjml-content>');
  process.exit(1);
}

const mjmlContent = process.argv[2];

try {
  const result = mjml(mjmlContent, {
    validationLevel: 'soft',
    minify: true
  });

  if (result.errors.length > 0) {
    console.error('MJML Errors:', result.errors);
  }

  process.stdout.write(result.html);
} catch (error) {
  console.error('Error compiling MJML:', error.message);
  process.exit(1);
}
