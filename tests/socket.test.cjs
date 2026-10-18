const {test} = require('node:test');
const assert = require('node:assert/strict');
const {EventEmitter} = require('node:events');
const {readFileSync} = require('node:fs');
const {runInNewContext} = require('node:vm');

function harness() {
    const connections = [], timers = [], flashes = [];
    class WebSocket {
        static OPEN = 1;
        readyState = 0;
        sent = [];
        constructor(url) { this.url = url; connections.push(this); }
        send(value) { this.sent.push(JSON.parse(value).controls); }
        open() { this.readyState = 1; this.onopen(); }
        close() { this.readyState = 3; this.onclose(); }
    }
    const source = readFileSync('ui/api/socket.js', 'utf8')
        .replace('import EventEmitter from "events";', '')
        .replace('export default bus;', 'result.bus = bus;');
    const result = {};
    runInNewContext(source, {
        EventEmitter, WebSocket, result,
        window: {location: {protocol: 'https:', host: 'example.test'}, flash: (...args) => flashes.push(args)},
        setTimeout: fn => timers.push(fn),
    });
    return {bus: result.bus, connections, timers, flashes};
}

test('subscriptions requested before open survive initial connection and reconnect', () => {
    const {bus, connections, timers} = harness();
    bus.emit('server status subscribe');
    bus.emit('log subscribe');
    assert.equal(connections[0].sent.length, 0);
    connections[0].open();
    assert.equal(connections[0].url, 'wss://example.test/ws');
    assert.deepEqual(connections[0].sent.map(x => x.value), ['server_status', 'gamelog']);
    connections[0].close();
    bus.emit('log unsubscribe');
    timers.shift()();
    connections[1].open();
    assert.deepEqual(connections[1].sent, [{type: 'subscribe', value: 'server_status'}]);
    assert.equal(bus.listenerCount('server status subscribe'), 1);
});

test('subscriptions are deduplicated and received status reaches listeners', () => {
    const {bus, connections} = harness();
    connections[0].open();
    bus.emit('server status subscribe');
    bus.emit('server status subscribe');
    assert.equal(connections[0].sent.length, 1);
    let status;
    bus.on('server_status', value => { status = JSON.parse(value); });
    connections[0].onmessage({data: JSON.stringify({room_name: 'server_status', message: '{"running":true}'})});
    assert.equal(status.running, true);
    bus.emit('server status unsubscribe');
    assert.equal(connections[0].sent[1].type, 'unsubscribe');
});

test('disconnected commands report an error and are never replayed', () => {
    const {bus, connections, flashes} = harness();
    bus.emit('command send', '/server-save');
    assert.equal(flashes.length, 1);
    connections[0].open();
    assert.equal(connections[0].sent.length, 0);
});
