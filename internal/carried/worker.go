package carried

import (
	"bufio"
	"encoding/json"
	"io"
	"regexp"
	"runtime/debug"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

const (
	maxResultBytes  = 768 << 10
	maxWorkerOutput = 1 << 20
	maxStatusText   = 512
	maxErrorText    = 4 << 10
	maxCallStack    = 10000

	exitBadStart   = 2
	exitParentGone = 3
)

// callCodes are the refusals a host call rejects with. One the program does
// not catch ends the run with the same code.
var callCodes = map[string]bool{
	CodeCallRefused: true, CodeCallInvalid: true, CodeNotFound: true, CodeNotAFile: true,
	CodeTooLarge: true, CodeLimit: true, CodeGitFailed: true, CodeFetchRefused: true,
	CodeFetchFailed: true, CodeOutputRefused: true, CodeFolderUnreadable: true,
	CodeProgramInvalid: true, CodeNotText: true,
}

var moduleSyntax = regexp.MustCompile(`(?m)^\s*(import|export)\b`)

type pendingCall struct {
	resolve, reject func(any) error
}

type worker struct {
	out     *lineWriter
	loop    *eventloop.EventLoop
	vm      *goja.Runtime
	entry   string
	modules map[string]string
	cache   map[string]goja.Value
	pending map[int64]pendingCall
	next    int64
	printed int
	done    chan int
	then    goja.Callable
	toJSON  goja.Callable
	parse   goja.Callable
}

// RunWorker is a program worker's whole life: it reads the start message
// from in, runs the program's main, and writes its calls, output and outcome
// to out. It returns the process's exit code. It returns at once when in
// ends, because the app that started it is gone.
func RunWorker(in io.Reader, out io.Writer) int {
	r := bufio.NewReaderSize(in, 64<<10)
	line, err := readLine(r, maxStartLine)
	if err != nil {
		return exitBadStart
	}
	var start startMessage
	if err := json.Unmarshal(line, &start); err != nil || start.Modules == nil {
		return exitBadStart
	}
	if _, ok := start.Modules[start.Entry]; !ok {
		return exitBadStart
	}
	limitMemory(start.Memory)
	w := &worker{
		out:     &lineWriter{w: out},
		entry:   start.Entry,
		modules: start.Modules,
		cache:   map[string]goja.Value{},
		pending: map[int64]pendingCall{},
		done:    make(chan int, 2),
	}
	w.loop = eventloop.NewEventLoop(eventloop.EnableConsole(false))
	w.loop.Start()
	go w.replies(r)
	w.loop.RunOnLoop(func(vm *goja.Runtime) { w.begin(vm, start) })
	return <-w.done
}

// limitMemory makes the worker's own collector work harder near the cap and,
// where the system has one, sets a hard limit the kernel enforces. The app's
// watch from outside is what holds the cap; these only make it hold sooner.
func limitMemory(memory uint64) {
	if memory == 0 {
		return
	}
	debug.SetMemoryLimit(int64(memory / 4 * 3))
	hardLimit(memory * 2)
}

func (w *worker) finish(msg workerMessage) {
	w.out.send(msg)
	select {
	case w.done <- 0:
	default:
	}
}

func (w *worker) replies(r *bufio.Reader) {
	for {
		line, err := readLine(r, maxReplyLine)
		if err != nil {
			select {
			case w.done <- exitParentGone:
			default:
			}
			return
		}
		var msg replyMessage
		if json.Unmarshal(line, &msg) != nil || msg.T != "reply" {
			continue
		}
		w.loop.RunOnLoop(func(vm *goja.Runtime) { w.settleCall(msg) })
	}
}

func (w *worker) begin(vm *goja.Runtime, start startMessage) {
	w.vm = vm
	vm.SetMaxCallStackSize(maxCallStack)
	helpers, err := vm.RunString(`[
		function (p, ok, bad) { Promise.resolve(p).then(ok, bad) },
		function (v) { return JSON.stringify(v) },
		function (s) { return JSON.parse(s) },
	]`)
	if err != nil {
		w.finish(workerMessage{T: "error", Code: CodeWorkerFailed, Message: err.Error()})
		return
	}
	list := helpers.ToObject(vm)
	w.then, _ = goja.AssertFunction(list.Get("0"))
	w.toJSON, _ = goja.AssertFunction(list.Get("1"))
	w.parse, _ = goja.AssertFunction(list.Get("2"))
	clay := w.install(vm)

	program, err := goja.Compile(start.Entry+".js", start.Modules[start.Entry], false)
	if err != nil {
		w.finish(workerMessage{T: "error", Code: CodeProgramInvalid, Message: syntaxText(start.Entry, start.Modules[start.Entry], err)})
		return
	}
	if _, err := vm.RunProgram(program); err != nil {
		w.thrown(err)
		return
	}
	main, ok := goja.AssertFunction(vm.Get("main"))
	if !ok {
		w.finish(workerMessage{T: "error", Code: CodeProgramInvalid, Message: "This program has no main function. Write async function main(input, clay) { ... } in it."})
		return
	}
	input := goja.Undefined()
	if len(start.Input) > 0 {
		if input, err = w.parse(goja.Undefined(), vm.ToValue(string(start.Input))); err != nil {
			w.thrown(err)
			return
		}
	}
	ret, err := main(goja.Undefined(), input, clay)
	if err != nil {
		w.thrown(err)
		return
	}
	w.then(goja.Undefined(), ret,
		vm.ToValue(func(call goja.FunctionCall) goja.Value { w.result(call.Argument(0)); return goja.Undefined() }),
		vm.ToValue(func(call goja.FunctionCall) goja.Value { w.rejected(call.Argument(0)); return goja.Undefined() }))
}

// install sets the clay object, console and require. main gets clay as its
// second argument; it is a global too, so a required module reaches it.
func (w *worker) install(vm *goja.Runtime) *goja.Object {
	files := vm.NewObject()
	for _, name := range []string{"read", "list", "write"} {
		files.Set(name, w.hostCall("files."+name))
	}
	git := vm.NewObject()
	for _, name := range []string{"log", "diff", "branches", "status", "show"} {
		git.Set(name, w.hostCall("git."+name))
	}
	clay := vm.NewObject()
	clay.Set("files", files)
	clay.Set("git", git)
	clay.Set("fetch", w.hostCall("fetch"))
	clay.Set("output", w.hostCall("output"))
	clay.Set("status", func(call goja.FunctionCall) goja.Value {
		w.out.send(workerMessage{T: "status", Text: clip(call.Argument(0).String(), maxStatusText)})
		return goja.Undefined()
	})
	vm.Set("clay", clay)
	console := vm.NewObject()
	for _, name := range []string{"log", "info", "warn", "error", "debug"} {
		console.Set(name, w.print)
	}
	vm.Set("console", console)
	vm.Set("require", w.require)
	if wrapTimers, err := vm.RunString(`(function (onError) {
		for (const name of ["setTimeout", "setInterval", "setImmediate"]) {
			const orig = globalThis[name];
			if (typeof orig !== "function") continue;
			globalThis[name] = function (fn, ...rest) {
				if (typeof fn !== "function") return orig.call(this, fn, ...rest);
				return orig.call(this, function (...args) {
					try {
						return fn.apply(this, args);
					} catch (err) {
						onError(err);
					}
				}, ...rest);
			};
		}
	})`); err == nil {
		if fn, ok := goja.AssertFunction(wrapTimers); ok {
			_, _ = fn(goja.Undefined(), vm.ToValue(func(call goja.FunctionCall) goja.Value {
				w.rejected(call.Argument(0))
				return goja.Undefined()
			}))
		}
	}
	return clay
}

// hostCall is one clay function: it sends the call to the app and returns a
// promise its reply settles.
func (w *worker) hostCall(name string) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		vm := w.vm
		p, resolve, reject := vm.NewPromise()
		args := make([]any, len(call.Arguments))
		for i, a := range call.Arguments {
			args[i] = a
		}
		raw, err := w.toJSON(goja.Undefined(), vm.NewArray(args...))
		switch {
		case err != nil:
			_ = reject(w.errorValue(&Error{Code: CodeCallInvalid, Message: "The arguments of " + name + " cannot be sent to HTML Clay: " + clip(err.Error(), 200)}))
			return vm.ToValue(p)
		case len(raw.String()) > maxWorkerLine-4096:
			_ = reject(w.errorValue(&Error{Code: CodeTooLarge, Message: "The arguments of " + name + " are too large."}))
			return vm.ToValue(p)
		}
		w.next++
		w.pending[w.next] = pendingCall{resolve: resolve, reject: reject}
		w.out.send(workerMessage{T: "call", ID: w.next, Name: name, Args: json.RawMessage(raw.String())})
		return vm.ToValue(p)
	}
}

func (w *worker) settleCall(msg replyMessage) {
	call, ok := w.pending[msg.ID]
	if !ok {
		return
	}
	delete(w.pending, msg.ID)
	if msg.Error != nil {
		_ = call.reject(w.errorValue(msg.Error))
		return
	}
	value := goja.Null()
	if len(msg.Value) > 0 {
		v, err := w.parse(goja.Undefined(), w.vm.ToValue(string(msg.Value)))
		if err != nil {
			_ = call.reject(w.errorValue(&Error{Code: CodeWorkerFailed, Message: "HTML Clay's answer could not be read."}))
			return
		}
		value = v
	}
	_ = call.resolve(value)
}

func (w *worker) errorValue(e *Error) goja.Value {
	obj, err := w.vm.New(w.vm.Get("Error"), w.vm.ToValue(e.Message))
	if err != nil {
		return w.vm.ToValue(e.Message)
	}
	obj.Set("code", e.Code)
	return obj
}

func (w *worker) print(call goja.FunctionCall) goja.Value {
	if w.printed >= maxWorkerOutput {
		return goja.Undefined()
	}
	parts := make([]string, len(call.Arguments))
	for i, a := range call.Arguments {
		if s, ok := a.Export().(string); ok {
			parts[i] = s
			continue
		}
		if j, err := w.toJSON(goja.Undefined(), a); err == nil && !goja.IsUndefined(j) {
			parts[i] = j.String()
		} else {
			parts[i] = a.String()
		}
	}
	text := strings.Join(parts, " ") + "\n"
	w.printed += len(text)
	w.out.send(workerMessage{T: "log", Text: text})
	return goja.Undefined()
}

// require loads another program tag of the same document, by name, from the
// stored copy the run was given. Nothing else can be required: no file, no
// package and no module of the host.
func (w *worker) require(call goja.FunctionCall) goja.Value {
	vm := w.vm
	name := strings.TrimSuffix(strings.TrimPrefix(call.Argument(0).String(), "./"), ".js")
	if v, ok := w.cache[name]; ok {
		return v
	}
	text, ok := w.modules[name]
	if !ok || name == w.entry {
		panic(vm.NewTypeError("This page has no program named " + clip(name, 64) + " to require."))
	}
	module := vm.NewObject()
	exports := vm.NewObject()
	module.Set("exports", exports)
	compiled, err := goja.Compile(name+".js", "(function (exports, require, module) {"+text+"\n})", false)
	if err != nil {
		panic(w.errorValue(&Error{Code: CodeProgramInvalid, Message: syntaxText(name, text, err)}))
	}
	wrapped, err := vm.RunProgram(compiled)
	if err != nil {
		panic(err)
	}
	fn, _ := goja.AssertFunction(wrapped)
	w.cache[name] = exports
	if _, err := fn(goja.Undefined(), exports, vm.Get("require"), module); err != nil {
		delete(w.cache, name)
		panic(err)
	}
	result := module.Get("exports")
	w.cache[name] = result
	return result
}

func (w *worker) result(v goja.Value) {
	raw, err := w.toJSON(goja.Undefined(), v)
	if err != nil {
		w.finish(workerMessage{T: "error", Code: CodeProgramFailed, Message: "The program's result is not JSON: " + clip(err.Error(), 200)})
		return
	}
	text := "null"
	if !goja.IsUndefined(raw) {
		text = raw.String()
	}
	if len(text) > maxResultBytes {
		w.finish(workerMessage{T: "error", Code: CodeResultTooLarge, Message: "The program's result is larger than 768 KB. Write large output with clay.output instead."})
		return
	}
	w.finish(workerMessage{T: "done", Value: json.RawMessage(text)})
}

func (w *worker) rejected(v goja.Value) {
	code := CodeProgramFailed
	if obj, ok := v.(*goja.Object); ok {
		if c := obj.Get("code"); c != nil {
			if s, ok := c.Export().(string); ok && callCodes[s] {
				code = s
			}
		}
	}
	w.finish(workerMessage{T: "error", Code: code, Message: clip(v.String(), maxErrorText)})
}

// syntaxText explains a program that does not compile. goja has no ES
// modules, so a program written with import or export gets the way that works.
func syntaxText(name, text string, err error) string {
	msg := "The program " + name + " has a syntax error: " + clip(err.Error(), 400)
	if moduleSyntax.MatchString(text) {
		msg += ` Programs cannot use import or export. Set module.exports in another program in this page and load it with require("./name").`
	}
	return msg
}

func (w *worker) thrown(err error) {
	if ex, ok := err.(*goja.Exception); ok {
		w.rejected(ex.Value())
		return
	}
	w.finish(workerMessage{T: "error", Code: CodeProgramFailed, Message: clip(err.Error(), maxErrorText)})
}
