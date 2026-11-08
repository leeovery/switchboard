// Package forecast says where each account's week runs out: the pool's use
// played forward, a step at a time, through the router's own choice of
// account, by the rates its use has run at lately, the hold curve that keeps
// use on the account it's on, and the accounts' sizes. It's for showing
// alone: the router never routes by it. Every function is pure and is handed
// the clock.
package forecast
