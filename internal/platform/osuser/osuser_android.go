package osuser

// Name is empty on Android: every app runs as its own uid, whose name says nothing
// about the person, and the device label already names the phone.
func Name() string { return "" }
