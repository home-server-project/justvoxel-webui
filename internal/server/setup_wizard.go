package server

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/home-server-project/justvoxel-webui/internal/api"
)

const setupDraftLifetime = 24 * time.Hour

var setupMemoryPattern = regexp.MustCompile(`^([1-9][0-9]*)([mMgG])$`)
var setupImageTagPattern = regexp.MustCompile