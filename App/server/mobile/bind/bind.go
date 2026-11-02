// Package bind is the gomobile binding workspace for the HENU parser AAR.
// It exists only so `gomobile bind kairos/server/mobile/henuparse` can run
// with golang.org/x/mobile as a module requirement WITHOUT adding that
// requirement to the production server module's go.mod. The bound package is
// the henuparse wrapper one level up; nothing is exported from here.
package bind
