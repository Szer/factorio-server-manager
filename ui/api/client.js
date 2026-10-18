import Axios from "axios";

const client = Axios.create({
    withCredentials: true,
    headers: {
        'Content-Type': 'application/json'
    }
});

client.interceptors.response.use(res => res, err => {
    if (err.config?.silent || Axios.isCancel(err)) return Promise.reject(err);
    if (!err.response) {
        window.flash("Request failed or timed out. Check the server status before retrying.", "red");
    } else if(err.response.status === 502) {
        window.flash("Service not available", "red");
    } else if (err.response.status !== 401) {
        window.flash(err.response.data, "red");
    }
    return Promise.reject(err);
});

export default client;