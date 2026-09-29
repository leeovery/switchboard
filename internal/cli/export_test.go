package cli

// RoleAnnotation is the annotation a command logs its role under, for tests
// that add a command of their own.
const RoleAnnotation = roleAnnotation

// ReadUnseen is how HiddenInput reads, given the terminal's part in it, for
// tests of how a read the user interrupts ends.
var ReadUnseen = readUnseen
