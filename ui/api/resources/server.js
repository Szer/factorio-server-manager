import client from "../client";

export default {
    factorioVersion: async () => {
        const response = await client.get('/api/server/facVersion');
        return response.data;
    },
    status: async (options = {}) => {
        const response = await client.get('/api/server/status', {timeout: 10000, ...options});
        return response.data;
    },
    stop: async () => {
        const response = await client.get('/api/server/stop', {timeout: 10000});
        return response.data;
    },
    start: async (ip, port, savefile) => {
        const response = await client.post('/api/server/start', {
            bindip: ip,
            savefile,
            port
        }, {timeout: 10000});
        return response.data;
    },
    kill: async () => {
        const response = await client.get('/api/server/kill', {timeout: 10000});
        return response.data;
    }
}