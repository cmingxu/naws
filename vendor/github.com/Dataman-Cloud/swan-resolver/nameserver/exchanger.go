package nameserver

import (
	"time"

	"github.com/miekg/dns"
)

// Exchanger is an interface capturing a dns.Client Exchange method.
type Exchanger interface {
	// Exchange performs an synchronous query. It sends the message m to the address
	// contained in addr (host:port) and waits for a reply.
	Exchange(m *dns.Msg, addr string) (r *dns.Msg, rtt time.Duration, err error)
}

// Func is a function type that implements the Exchanger interface.
type Func func(*dns.Msg, string) (*dns.Msg, time.Duration, error)

// Exchange implements the Exchanger interface.
func (f Func) Exchange(m *dns.Msg, addr string) (*dns.Msg, time.Duration, error) {
	return f(m, addr)
}

// A Decorator adds a layer of behaviour to a given Exchanger.
type Decorator func(Exchanger) Exchanger

// IgnoreErrTruncated is a Decorator which causes dns.ErrTruncated to be ignored
var IgnoreErrTruncated Decorator = func(ex Exchanger) Exchanger {
	return Func(func(m *dns.Msg, a string) (r *dns.Msg, rtt time.Duration, err error) {
		r, rtt, err = ex.Exchange(m, a)
		if err == dns.ErrTruncated {
			// Ignore
			err = nil
		}
		return
	})
}

// Decorate decorates an Exchanger with the given Decorators.
func Decorate(ex Exchanger, ds ...Decorator) Exchanger {
	decorated := ex
	for _, decorate := range ds {
		decorated = decorate(decorated)
	}
	return decorated
}
