import EventEmitter from "events";

const bus = new EventEmitter();
const subscriptions = new Set();
const scheme = window.location.protocol === "https:" ? "wss" : "ws";
let socket;

function send(type, value) {
    if (socket?.readyState !== WebSocket.OPEN) return false;
    socket.send(JSON.stringify({room_name: "", controls: {type, value}}));
    return true;
}

function subscribe(room) {
    if (subscriptions.has(room)) return;
    subscriptions.add(room);
    send('subscribe', room);
}

function unsubscribe(room) {
    subscriptions.delete(room);
    send('unsubscribe', room);
}

bus.on('log subscribe', () => subscribe('gamelog'));
bus.on('log unsubscribe', () => unsubscribe('gamelog'));
bus.on('server status subscribe', () => subscribe('server_status'));
bus.on('server status unsubscribe', () => unsubscribe('server_status'));
bus.on('command send', command => {
    if (!send('command', command)) window.flash('Console disconnected. Try again after reconnecting.', 'red');
});

function connect() {
    socket = new WebSocket(`${scheme}://${window.location.host}/ws`);
    socket.onopen = () => {
        // Replay desired subscriptions after both initial connection and reconnect.
        subscriptions.forEach(room => send('subscribe', room));
        bus.emit('connected');
    };
    socket.onmessage = event => {
        const {room_name, message} = JSON.parse(event.data);
        bus.emit(room_name, message);
    };
    socket.onerror = () => socket.close();
    socket.onclose = () => setTimeout(connect, 5000);
}

connect();
export default bus;
