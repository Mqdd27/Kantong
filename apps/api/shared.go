package main

import "errors"

// handOffShared runs before an account is deleted: its kantong bersama pass to their oldest member.
func (a *app) handOffShared(uid int64) error { return errors.New("not implemented") }
