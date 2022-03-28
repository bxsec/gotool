package netx

import (
	"bufio"
	"context"
	"crypto/tls"
	"github.com/bxsec/gotool/log"
	"github.com/bxsec/gotool/protocol"
	"github.com/bxsec/gotool/share"
	"io"
	"net"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)


