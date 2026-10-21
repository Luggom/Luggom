// Sem consola no Windows em produção.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    luggom_desktop::run();
}
